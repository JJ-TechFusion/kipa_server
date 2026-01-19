package service

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/integration/storage"
)

type UploadService interface {
	UploadProfilePicture(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (string, error)
	UploadItemImage(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (string, error)
	UploadRiderDocument(ctx context.Context, userID uuid.UUID, docType string, file *multipart.FileHeader) (string, error)
}

type UploadServiceImpl struct {
	s3Client *storage.S3Client
	logger   *zap.Logger
}

var validRiderDocTypes = map[string]bool{
	"license": true,
	"nin":     true,
	"face":    true,
	"vehicle": true,
}

func NewUploadService(s3Client *storage.S3Client, logger *zap.Logger) UploadService {
	return &UploadServiceImpl{
		s3Client: s3Client,
		logger:   logger,
	}
}

func (s *UploadServiceImpl) UploadProfilePicture(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (string, error) {
	return s.uploadFile(ctx, file, "profiles")
}

func (s *UploadServiceImpl) UploadItemImage(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (string, error) {
	return s.uploadFile(ctx, file, "items")
}

func (s *UploadServiceImpl) UploadRiderDocument(ctx context.Context, userID uuid.UUID, docType string, file *multipart.FileHeader) (string, error) {
	if !validRiderDocTypes[docType] {
		return "", fmt.Errorf("invalid document type: %s", docType)
	}
	folder := fmt.Sprintf("riders/%s", docType)
	return s.uploadFile(ctx, file, folder)
}

func (s *UploadServiceImpl) uploadFile(ctx context.Context, file *multipart.FileHeader, folder string) (string, error) {
	if !s.s3Client.IsConfigured() {
		return "", fmt.Errorf("file upload not configured")
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	contentType := file.Header.Get("Content-Type")
	if !storage.IsAllowedMimeType(contentType) {
		return "", fmt.Errorf("file type not allowed: %s", contentType)
	}

	url, err := s.s3Client.Upload(ctx, src, folder, contentType, file.Size)
	if err != nil {
		s.logger.Error("failed to upload file",
			zap.String("folder", folder),
			zap.Error(err),
		)
		return "", err
	}

	s.logger.Info("file uploaded",
		zap.String("folder", folder),
		zap.String("url", url),
	)

	return url, nil
}

var _ UploadService = (*UploadServiceImpl)(nil)
