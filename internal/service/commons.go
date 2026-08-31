package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const maxFileSize = 5 * 1024 * 1024 // 5MB

var uploadTypeConfig = map[string]struct {
	directory string
	maxBytes  int64
}{
	"project_image":      {directory: "project-images", maxBytes: 1 * 1024 * 1024},
	"talent_work":        {directory: "talent-work-images", maxBytes: 1 * 1024 * 1024},
	"milestone_evidence": {directory: "milestone-evidence", maxBytes: 300 * 1024},
}

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
	return s.uploadFileTo(file, header, "", maxFileSize)
}

// UploadBusinessImage validates stricter per-purpose limits and isolates OSS keys.
func (s *CommonsService) UploadBusinessImage(file multipart.File, header *multipart.FileHeader, uploadType string) (*oss.UploadResult, error) {
	config, ok := uploadTypeConfig[uploadType]
	if !ok {
		return nil, ErrBadRequest("无效的文件类型")
	}
	return s.uploadFileTo(file, header, config.directory, config.maxBytes)
}

func (s *CommonsService) uploadFileTo(file multipart.File, header *multipart.FileHeader, directory string, sizeLimit int64) (*oss.UploadResult, error) {
	if header.Size > sizeLimit {
		return nil, ErrBadRequest(fmt.Sprintf("文件大小超过限制 (最大 %dKB)", sizeLimit/1024))
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		return nil, ErrBadRequest("不支持的文件类型，仅支持 JPG 和 PNG")
	}
	if err := validateImageContent(file, ext); err != nil {
		return nil, err
	}

	filename := uuid.New().String() + ext
	result, err := s.ossClient.UploadTo(file, directory, filename)
	if err != nil {
		log.Printf("[CommonsService.UploadFile] OSS upload error: %v", err)
		return nil, ErrInternal("文件上传失败")
	}
	return result, nil
}

// validateImageContent verifies the actual bytes instead of trusting the file
// extension. In particular, iOS may provide HEIC data carrying a .jpg name;
// such a file uploads successfully but cannot be rendered by common browsers.
func validateImageContent(file multipart.File, ext string) error {
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return ErrBadRequest("无法读取图片，请重新选择后上传")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ErrBadRequest("无法读取图片，请重新选择后上传")
	}

	expectedType := "image/jpeg"
	if ext == ".png" {
		expectedType = "image/png"
	}
	if http.DetectContentType(header[:n]) != expectedType {
		return ErrBadRequest("请使用 JPG/PNG 格式的图片，不支持 HEIC/HEIF")
	}
	return nil
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
	// 1. 查询旧的 auth_img_url（允许用户尚未上传过，此时为 NULL）
	certInfo, err := s.userRepo.GetEduCertInfoByID(ctx, userID)
	if err != nil {
		log.Printf("[CommonsService.SubmitCertification] repository error getting cert info: %v", err)
		return nil, ErrInternal("获取旧认证图片失败")
	}
	var oldKey string
	if certInfo.AuthImgUrl != nil {
		oldKey = *certInfo.AuthImgUrl
	}

	// 2. 上传新文件
	result, err := s.UploadFile(file, header)
	if err != nil {
		return nil, err
	}

	// 3. 删除旧文件（上传成功后才删除，忽略删除失败）
	if oldKey != "" {
		_ = s.DeleteFile(oldKey)
	}

	// 4. 原子更新图片 URL，并在认证失败（status=2）时将状态重置为待审核（status=0）
	if err := s.userRepo.ResubmitCertification(ctx, userID, result.Key); err != nil {
		log.Printf("[CommonsService.SubmitCertification] repository error resubmitting certification: %v", err)
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
		log.Printf("[CommonsService.UploadAvatar] repository error getting user: %v", err)
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
		log.Printf("[CommonsService.UploadAvatar] repository error updating avatar: %v", err)
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
		log.Printf("[CommonsService.UploadCoverImage] repository error getting user: %v", err)
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
		log.Printf("[CommonsService.UploadCoverImage] repository error updating cover image: %v", err)
		return nil, ErrInternal("更新封面图失败")
	}

	return result, nil
}
