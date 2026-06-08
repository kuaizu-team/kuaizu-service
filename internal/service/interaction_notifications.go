package service

import (
	"context"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const subscribeInteractionRemark = "点击卡片查看详情"

type subscribeMessageSender interface {
	SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error
}

type subscribeNotification struct {
	ownerUserID int
	bizKey      string
	data        map[string]string
}

func notificationUserName(user *models.User) string {
	if user == nil || user.Nickname == nil {
		return "用户"
	}
	name := strings.TrimSpace(*user.Nickname)
	if name == "" {
		return "用户"
	}
	return truncate20(name)
}

func buildProjectInteractionNotification(kind string, operatorUserID int, project *models.Project, operatorName string) (subscribeNotification, bool) {
	if project == nil || operatorUserID <= 0 || operatorUserID == project.CreatorID {
		return subscribeNotification{}, false
	}

	bizKeyByKind := map[string]string{
		"like":     models.MsgBizKeyProjectLike,
		"favorite": models.MsgBizKeyProjectFavorite,
		"share":    models.MsgBizKeyProjectShare,
	}
	userFieldByKind := map[string]string{
		"like":     "like_user",
		"favorite": "favorite_user",
		"share":    "share_user",
	}

	bizKey, ok := bizKeyByKind[kind]
	if !ok {
		return subscribeNotification{}, false
	}

	data := map[string]string{
		"project_name":        truncate20(project.Name),
		userFieldByKind[kind]: truncate20(operatorName),
		"remark":              subscribeInteractionRemark,
	}
	return subscribeNotification{ownerUserID: project.CreatorID, bizKey: bizKey, data: data}, true
}

func buildTalentInteractionNotification(kind string, operatorUserID int, profile *models.TalentProfile, operatorName string) (subscribeNotification, bool) {
	if profile == nil || operatorUserID <= 0 || operatorUserID == profile.UserID {
		return subscribeNotification{}, false
	}

	bizKeyByKind := map[string]string{
		"like":     models.MsgBizKeyTalentLike,
		"favorite": models.MsgBizKeyTalentFavorite,
		"share":    models.MsgBizKeyTalentShare,
	}
	userFieldByKind := map[string]string{
		"like":     "like_user",
		"favorite": "favorite_user",
		"share":    "share_user",
	}

	bizKey, ok := bizKeyByKind[kind]
	if !ok {
		return subscribeNotification{}, false
	}

	data := map[string]string{
		userFieldByKind[kind]: truncate20(operatorName),
		"remark":              subscribeInteractionRemark,
	}
	return subscribeNotification{ownerUserID: profile.UserID, bizKey: bizKey, data: data}, true
}

func buildProjectVisitNotification(viewerUserID int, project *models.Project, viewerName string) (subscribeNotification, bool) {
	if project == nil || viewerUserID <= 0 || viewerUserID == project.CreatorID {
		return subscribeNotification{}, false
	}
	return subscribeNotification{
		ownerUserID: project.CreatorID,
		bizKey:      models.MsgBizKeyProjectVisit,
		data: map[string]string{
			"project_name": truncate20(project.Name),
			"visit_user":   truncate20(viewerName),
			"remark":       subscribeInteractionRemark,
		},
	}, true
}

func buildTalentVisitNotification(viewerUserID int, profile *models.TalentProfile, viewerName string) (subscribeNotification, bool) {
	if profile == nil || viewerUserID <= 0 || viewerUserID == profile.UserID {
		return subscribeNotification{}, false
	}
	return subscribeNotification{
		ownerUserID: profile.UserID,
		bizKey:      models.MsgBizKeyTalentVisit,
		data: map[string]string{
			"visit_user": truncate20(viewerName),
			"remark":     subscribeInteractionRemark,
		},
	}, true
}

func (s *InteractionService) notifyInteractionAsync(ctx context.Context, target, kind string, targetID, operatorUserID int) {
	if s.message == nil {
		return
	}

	go func(asyncCtx context.Context) {
		operator, err := s.repo.User.GetByID(asyncCtx, operatorUserID)
		if err != nil {
			log.Printf("[Interaction.notifyInteractionAsync] get operator error (non-fatal): %v", err)
		}
		operatorName := notificationUserName(operator)

		var (
			notification subscribeNotification
			ok           bool
		)
		switch target {
		case repository.InteractionProject:
			project, err := s.repo.Project.GetByID(asyncCtx, targetID)
			if err != nil {
				log.Printf("[Interaction.notifyInteractionAsync] get project error (non-fatal): %v", err)
				return
			}
			notification, ok = buildProjectInteractionNotification(kind, operatorUserID, project, operatorName)
		case repository.InteractionTalent:
			profile, err := s.repo.TalentProfile.GetByID(asyncCtx, targetID)
			if err != nil {
				log.Printf("[Interaction.notifyInteractionAsync] get talent profile error (non-fatal): %v", err)
				return
			}
			notification, ok = buildTalentInteractionNotification(kind, operatorUserID, profile, operatorName)
		default:
			return
		}
		if !ok {
			return
		}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, notification.ownerUserID, notification.bizKey, notification.data); err != nil {
			log.Printf("[Interaction.notifyInteractionAsync] send subscribe message error (non-fatal): %v", err)
		}
	}(context.WithoutCancel(ctx))
}

func sendSubscribeNotification(ctx context.Context, sender subscribeMessageSender, notification subscribeNotification) {
	if sender == nil {
		return
	}
	if err := sender.SendSubscribeMsgByBizKey(ctx, notification.ownerUserID, notification.bizKey, notification.data); err != nil {
		log.Printf("[sendSubscribeNotification] send subscribe message error (non-fatal): %v", err)
	}
}
