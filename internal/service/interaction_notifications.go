package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const subscribeInteractionRemark = "点击卡片查看详情"

type subscribeMessageSender interface {
	SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error
}

type subscribeMessagePageSender interface {
	SendSubscribeMsgByBizKeyWithPage(ctx context.Context, userID int, bizKey string, businessData map[string]string, pagePath string) error
}

type subscribeNotification struct {
	ownerUserID int
	bizKey      string
	data        map[string]string
	pagePath    string
}

func projectDashboardPagePath(projectID int) string {
	return fmt.Sprintf("pages/project-dashboard/project-dashboard?id=%d", projectID)
}

func talentDashboardPagePath(talentID int) string {
	return fmt.Sprintf("pages/talent-dashboard/talent-dashboard?id=%d", talentID)
}

func notificationUserName(user *models.User) string {
	if user == nil {
		return models.DefaultUserNickname
	}
	name := *models.DisplayNickname(user.Nickname)
	return truncate20WithEllipsis(name)
}

func notificationGroupUserName(userName string, count int) string {
	suffix := "等" + strconv.Itoa(count) + "人"
	maxNameLen := 20 - len([]rune(suffix))
	if maxNameLen < 1 {
		return truncate20WithEllipsis(suffix)
	}
	name := strings.TrimSpace(userName)
	if name == "" {
		name = models.DefaultUserNickname
	}
	runes := []rune(name)
	if len(runes) > maxNameLen {
		if maxNameLen > 3 {
			name = string(runes[:maxNameLen-3]) + "..."
		} else {
			name = string(runes[:maxNameLen])
		}
	}
	return name + suffix
}

func buildProjectInteractionNotification(kind string, operatorUserID int, project *models.Project, operatorName string, count int) (subscribeNotification, bool) {
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
		userFieldByKind[kind]: notificationInteractionUserName(kind, operatorName, count),
		"remark":              subscribeInteractionRemark,
	}
	return subscribeNotification{
		ownerUserID: project.CreatorID,
		bizKey:      bizKey,
		data:        data,
		pagePath:    projectDashboardPagePath(project.ID),
	}, true
}

func buildTalentInteractionNotification(kind string, operatorUserID int, profile *models.TalentProfile, operatorName string, count int) (subscribeNotification, bool) {
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
		userFieldByKind[kind]: notificationInteractionUserName(kind, operatorName, count),
		"remark":              subscribeInteractionRemark,
	}
	return subscribeNotification{
		ownerUserID: profile.UserID,
		bizKey:      bizKey,
		data:        data,
		pagePath:    talentDashboardPagePath(profile.ID),
	}, true
}

func notificationInteractionUserName(kind, operatorName string, count int) string {
	if count >= 3 && kind != "favorite" {
		return notificationGroupUserName(operatorName, count)
	}
	return truncate20WithEllipsis(operatorName)
}

func buildProjectVisitNotification(viewerUserID int, project *models.Project, viewerName string, count int) (subscribeNotification, bool) {
	if project == nil || viewerUserID <= 0 || viewerUserID == project.CreatorID {
		return subscribeNotification{}, false
	}
	return subscribeNotification{
		ownerUserID: project.CreatorID,
		bizKey:      models.MsgBizKeyProjectVisit,
		data: map[string]string{
			"project_name": truncate20(project.Name),
			"visit_user":   notificationGroupUserName(viewerName, count),
			"remark":       subscribeInteractionRemark,
		},
		pagePath: projectDashboardPagePath(project.ID),
	}, true
}

func buildTalentVisitNotification(viewerUserID int, profile *models.TalentProfile, viewerName string, count int) (subscribeNotification, bool) {
	if profile == nil || viewerUserID <= 0 || viewerUserID == profile.UserID {
		return subscribeNotification{}, false
	}
	return subscribeNotification{
		ownerUserID: profile.UserID,
		bizKey:      models.MsgBizKeyTalentVisit,
		data: map[string]string{
			"visit_user": notificationGroupUserName(viewerName, count),
			"remark":     subscribeInteractionRemark,
		},
		pagePath: talentDashboardPagePath(profile.ID),
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
			count := 1
			if kind != "favorite" {
				progress, err := s.repo.Interaction.NotifyProgress(asyncCtx, target, kind, targetID, operatorUserID, project.CreatorID)
				if err != nil {
					log.Printf("[Interaction.notifyInteractionAsync] get project notify progress error (non-fatal): %v", err)
					return
				}
				if !shouldSendGroupedInteractionNotification(progress) {
					return
				}
				count = progress.DistinctUserCount
			}
			notification, ok = buildProjectInteractionNotification(kind, operatorUserID, project, operatorName, count)
		case repository.InteractionTalent:
			profile, err := s.repo.TalentProfile.GetByID(asyncCtx, targetID)
			if err != nil {
				log.Printf("[Interaction.notifyInteractionAsync] get talent profile error (non-fatal): %v", err)
				return
			}
			count := 1
			if kind != "favorite" {
				progress, err := s.repo.Interaction.NotifyProgress(asyncCtx, target, kind, targetID, operatorUserID, profile.UserID)
				if err != nil {
					log.Printf("[Interaction.notifyInteractionAsync] get talent notify progress error (non-fatal): %v", err)
					return
				}
				if !shouldSendGroupedInteractionNotification(progress) {
					return
				}
				count = progress.DistinctUserCount
			}
			notification, ok = buildTalentInteractionNotification(kind, operatorUserID, profile, operatorName, count)
		default:
			return
		}
		if !ok {
			return
		}
		sendSubscribeNotification(asyncCtx, s.message, notification)
	}(context.WithoutCancel(ctx))
}

func shouldSendGroupedInteractionNotification(progress repository.InteractionNotifyProgress) bool {
	return progress.IsNewUser && progress.DistinctUserCount > 0 && progress.DistinctUserCount%3 == 0
}

func sendSubscribeNotification(ctx context.Context, sender subscribeMessageSender, notification subscribeNotification) {
	if sender == nil {
		return
	}
	if notification.pagePath != "" {
		if pageSender, ok := sender.(subscribeMessagePageSender); ok {
			if err := pageSender.SendSubscribeMsgByBizKeyWithPage(ctx, notification.ownerUserID, notification.bizKey, notification.data, notification.pagePath); err != nil {
				log.Printf("[sendSubscribeNotification] send subscribe message error (non-fatal): %v", err)
			}
			return
		}
	}
	if err := sender.SendSubscribeMsgByBizKey(ctx, notification.ownerUserID, notification.bizKey, notification.data); err != nil {
		log.Printf("[sendSubscribeNotification] send subscribe message error (non-fatal): %v", err)
	}
}
