package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	appconfig "kipa/internal/config"
)

var allowedMimeTypes = map[string]bool{
	"image/jpeg":          true,
	"image/jpg":           true,
	"image/png":           true,
	"image/webp":          true,
	"image/gif":           true,
	"image/heic":          true,
	"image/heif":          true,
	"image/heic-sequence": true,
	"image/heif-sequence": true,
}

var mimeToExt = map[string]string{
	"image/jpeg":          "jpg",
	"image/jpg":           "jpg",
	"image/png":           "png",
	"image/webp":          "webp",
	"image/gif":           "gif",
	"image/heic":          "heic",
	"image/heif":          "heif",
	"image/heic-sequence": "heic",
	"image/heif-sequence": "heif",
}

type S3Client struct {
	client        *s3.Client
	bucket        string
	baseURL       string
	maxFileSizeMB int
}

func NewS3Client(cfg *appconfig.S3Config) (*S3Client, error) {
	if cfg.Bucket == "" {
		return &S3Client{}, nil
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)

	return &S3Client{
		client:        client,
		bucket:        cfg.Bucket,
		baseURL:       cfg.BaseURL,
		maxFileSizeMB: cfg.MaxFileSizeMB,
	}, nil
}

func (c *S3Client) Upload(ctx context.Context, file io.Reader, folder, contentType string, fileSize int64) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("S3 client not configured")
	}

	if !allowedMimeTypes[contentType] {
		return "", fmt.Errorf("file type not allowed: %s", contentType)
	}

	maxBytes := int64(c.maxFileSizeMB * 1024 * 1024)
	if fileSize > maxBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed (%d MB)", c.maxFileSizeMB)
	}

	ext := mimeToExt[contentType]
	key := fmt.Sprintf("%s/%s.%s", folder, uuid.New().String(), ext)

	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(fileSize),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(c.baseURL, "/"), key)
	return url, nil
}

func (c *S3Client) Delete(ctx context.Context, fileURL string) error {
	if c.client == nil {
		return fmt.Errorf("S3 client not configured")
	}

	key := strings.TrimPrefix(fileURL, c.baseURL+"/")
	key = strings.TrimPrefix(key, c.baseURL)

	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

func (c *S3Client) IsConfigured() bool {
	return c.client != nil
}

func GetExtensionFromMime(contentType string) string {
	return mimeToExt[contentType]
}

func IsAllowedMimeType(contentType string) bool {
	return allowedMimeTypes[contentType]
}

func GetFileExtension(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}
