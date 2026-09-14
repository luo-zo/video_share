package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"

	"video_share/internal/config"
)

// Client 保存私有视频桶的访问配置，不将长期凭证暴露给调用方。
type Client struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

func NewClient(cfg *config.Config) (*Client, error) {
	if cfg == nil || cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
		return nil, fmt.Errorf("object storage credentials are required")
	}
	if err := s3utils.CheckValidBucketNameStrict(cfg.MinIOBucket); err != nil {
		return nil, fmt.Errorf("invalid object storage bucket: %w", err)
	}
	options := &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure:       cfg.MinIOUseSSL,
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	}
	client, err := minio.New(cfg.MinIOEndpoint, options)
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	publicEndpoint := cfg.MinIOPublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.MinIOEndpoint
	}
	presignClient, err := minio.New(publicEndpoint, options)
	if err != nil {
		return nil, fmt.Errorf("create object storage presign client: %w", err)
	}
	return &Client{client: client, presignClient: presignClient, bucket: cfg.MinIOBucket}, nil
}

// EnsureBucket 幂等创建私有桶，并允许多个 API 实例同时启动。
func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.client.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check video bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := c.client.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		if minio.ToErrorResponse(err).Code == "BucketAlreadyOwnedByYou" {
			return nil
		}
		return fmt.Errorf("create video bucket: %w", err)
	}
	return nil
}

// PresignPut 将 Content-Type 纳入签名；浏览器上传时必须发送相同的值。
// 预签名 PUT 不保证文件真实格式或大小，完成上传接口仍需核验对象。
func (c *Client) PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	if contentType != "video/mp4" {
		return "", fmt.Errorf("only video/mp4 uploads are supported")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", contentType)
	u, err := c.presignClient.PresignHeader(ctx, http.MethodPut, c.bucket, key, expiry, nil, headers)
	if err != nil {
		return "", fmt.Errorf("presign video upload: %w", err)
	}
	return u.String(), nil
}

func (c *Client) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return c.PresignGetAs(ctx, key, "video/mp4", expiry)
}

func (c *Client) PresignGetAs(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	params := make(url.Values)
	if contentType != "" {
		params.Set("response-content-type", contentType)
	}
	u, err := c.presignClient.PresignedGetObject(ctx, c.bucket, key, expiry, params)
	if err != nil {
		return "", fmt.Errorf("presign video playback: %w", err)
	}
	return u.String(), nil
}

func (c *Client) Download(ctx context.Context, key, destination string) error {
	object, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer object.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create downloaded file: %w", err)
	}
	_, copyErr := io.Copy(file, object)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("download object: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close downloaded file: %w", closeErr)
	}
	return nil
}

func (c *Client) UploadFile(ctx context.Context, key, source, contentType string) error {
	if _, err := c.client.FPutObject(ctx, c.bucket, key, source, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("upload object: %w", err)
	}
	return nil
}

func (c *Client) Read(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("maxBytes must be positive")
	}
	object, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("object exceeds read limit")
	}
	return data, nil
}

func (c *Client) Stat(ctx context.Context, key string) (size int64, contentType string, err error) {
	info, err := c.client.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("stat video object: %w", err)
	}
	return info.Size, info.ContentType, nil
}
