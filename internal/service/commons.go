package service

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/trv3wood/kuaizu-server/internal/oss"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

const maxFileSize = 5 * 1024 * 1024 // 5MB

var allowedExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}

// CommonsService handles common utilities like file upload.
type CommonsService struct {
	ossClient *oss.Client
	userRepo  repository.UserRepo
}

// NewCommonsService creates a new CommonsService.
func NewCommonsService(ossClient *oss.Client, userRepo repository.UserRepo) *CommonsService {
	return &CommonsService{ossClient: ossClient, userRepo: userRepo}
}

// UploadFile validates and uploads a multipart file to OSS.
func (s *CommonsService) UploadFile(file multipart.File, header *multipart.FileHeader) (*oss.UploadResult, error) {
	if header.Size > maxFileSize {
		return nil, ErrBadRequest(fmt.Sprintf("文件大小超过限制 (最大 %dMB)", maxFileSize/1024/1024))
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		return nil, ErrBadRequest("不支持的文件类型，仅支持 JPG 和 PNG")
	}

	filename := uuid.New().String() + ext
	result, err := s.ossClient.Upload(file, filename)
	if err != nil {
		return nil, ErrInternal("文件上传失败")
	}
	return result, nil
}

// DeleteFile removes a file from OSS by its key. Errors are logged but treated as
// non-fatal so they do not roll back an otherwise successful operation.
func (s *CommonsService) DeleteFile(key string) error {
	if key == "" {
		return nil
	}
	return s.ossClient.Delete(key)
}

// SubmitCertification uploads the new auth image, deletes the old one from OSS
// (if any), and updates the database with the new key. This is the single
// service-layer entry point for the certification image upload flow.
func (s *CommonsService) SubmitCertification(ctx context.Context, userID int, file multipart.File, header *multipart.FileHeader) (*oss.UploadResult, error) {
	// 1. 查询旧的 auth_img_url
	certInfo, err := s.userRepo.GetEduCertInfoByID(ctx, userID)
	if err != nil {
		return nil, ErrInternal("获取旧认证图片失败")
	}
	oldKey := certInfo.AuthImgUrl

	// 2. 上传新文件
	result, err := s.UploadFile(file, header)
	if err != nil {
		return nil, err
	}

	// 3. 删除旧文件（上传成功后才删除，忽略删除失败）
	if oldKey != "" {
		_ = s.DeleteFile(oldKey)
	}

	// 4. 更新数据库
	if err := s.userRepo.UpdateAuthImgUrl(ctx, userID, result.Key); err != nil {
		return nil, ErrInternal("更新认证图片失败")
	}

	return result, nil
}

// UploadAvatar uploads a new avatar for the user, deletes the old one from OSS,
// and persists the new URL to the database.
func (s *CommonsService) UploadAvatar(ctx context.Context, userID int, file multipart.File, header *multipart.FileHeader) (*oss.UploadResult, error) {
	// 1. 查询旧头像
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrInternal("获取用户信息失败")
	}

	// 2. 上传新文件
	result, err := s.UploadFile(file, header)
	if err != nil {
		return nil, err
	}

	// 3. 删除旧头像（忽略删除失败）
	if user != nil && user.AvatarUrl != nil && *user.AvatarUrl != "" {
		_ = s.DeleteFile(*user.AvatarUrl)
	}

	// 4. 更新数据库
	if err := s.userRepo.UpdateAvatarUrl(ctx, userID, result.Key); err != nil {
		return nil, ErrInternal("更新头像失败")
	}

	return result, nil
}

// UploadCoverImage uploads a new cover image for the user, deletes the old one
// from OSS, and persists the new URL to the database.
func (s *CommonsService) UploadCoverImage(ctx context.Context, userID int, file multipart.File, header *multipart.FileHeader) (*oss.UploadResult, error) {
	// 1. 查询旧封面图
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrInternal("获取用户信息失败")
	}

	// 2. 上传新文件
	result, err := s.UploadFile(file, header)
	if err != nil {
		return nil, err
	}

	// 3. 删除旧封面图（忽略删除失败）
	if user != nil && user.CoverImage != nil && *user.CoverImage != "" {
		_ = s.DeleteFile(*user.CoverImage)
	}

	// 4. 更新数据库
	if err := s.userRepo.UpdateCoverImage(ctx, userID, result.Key); err != nil {
		return nil, ErrInternal("更新封面图失败")
	}

	return result, nil
}
