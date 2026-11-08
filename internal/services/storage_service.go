package services

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"finsense-api/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageService struct {
	client    *minio.Client
	cfg       *config.Config
	publicURL string
}

func NewStorageService(cfg *config.Config) *StorageService {
	// Parse endpoint to separate host from scheme
	cleanEndpoint := cfg.S3Endpoint
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "http://")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")

	client, err := minio.New(cleanEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		log.Printf("[STORAGE WARNING] Failed to initialize MinIO client: %v", err)
	}

	return &StorageService{
		client:    client,
		cfg:       cfg,
		publicURL: cfg.S3PublicURL,
	}
}

func (s *StorageService) EnsureBucket(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("storage client is not initialized")
	}

	exists, err := s.client.BucketExists(ctx, s.cfg.S3Bucket)
	if err != nil {
		return fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.cfg.S3Bucket, minio.MakeBucketOptions{Region: s.cfg.S3Region})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Printf("[STORAGE] Bucket '%s' criado com sucesso.", s.cfg.S3Bucket)
	}

	return nil
}

func (s *StorageService) GeneratePresignedUploadURL(ctx context.Context, storageKey string, expires time.Duration) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("storage client is not initialized")
	}

	u, err := s.client.PresignedPutObject(ctx, s.cfg.S3Bucket, storageKey, expires)
	if err != nil {
		return "", err
	}

	return s.replacePublicHost(u), nil
}

func (s *StorageService) GeneratePresignedDownloadURL(ctx context.Context, storageKey string, expires time.Duration) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("storage client is not initialized")
	}

	reqParams := make(url.Values)
	u, err := s.client.PresignedGetObject(ctx, s.cfg.S3Bucket, storageKey, expires, reqParams)
	if err != nil {
		return "", err
	}

	return s.replacePublicHost(u), nil
}

// replacePublicHost replaces internal docker host (e.g. finsense-storage:9000) with client-accessible URL (e.g. localhost:9000)
func (s *StorageService) replacePublicHost(u *url.URL) string {
	if s.publicURL == "" {
		return u.String()
	}

	pub, err := url.Parse(s.publicURL)
	if err != nil {
		return u.String()
	}

	u.Scheme = pub.Scheme
	u.Host = pub.Host
	return u.String()
}

func (s *StorageService) VerifyObjectExists(ctx context.Context, storageKey string) (bool, int64, error) {
	if s.client == nil {
		return false, 0, fmt.Errorf("storage client is not initialized")
	}

	info, err := s.client.StatObject(ctx, s.cfg.S3Bucket, storageKey, minio.StatObjectOptions{})
	if err != nil {
		return false, 0, err
	}

	return true, info.Size, nil
}
