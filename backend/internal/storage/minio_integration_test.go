//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"video_share/internal/config"
)

func TestMinIOPresignedUploadAndPrivateRead(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("TEST_MINIO_ENDPOINT"))
	accessKey := os.Getenv("TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		if strings.EqualFold(os.Getenv("REQUIRE_INTEGRATION_TESTS"), "true") {
			t.Fatal("TEST_MINIO_ENDPOINT, TEST_MINIO_ACCESS_KEY and TEST_MINIO_SECRET_KEY are required")
		}
		t.Skip("MinIO integration endpoint or credentials not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	bucket := "video-share-test-" + uuid.NewString()
	client, err := NewClient(&config.Config{
		MinIOEndpoint: endpoint, MinIOPublicEndpoint: endpoint,
		MinIOAccessKey: accessKey, MinIOSecretKey: secretKey, MinIOBucket: bucket,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: "us-east-1", BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	const objectKey = "upload/signed.mp4"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.RemoveObject(cleanupCtx, bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
			t.Errorf("remove test object: %v", err)
		}
		if err := admin.RemoveBucket(cleanupCtx, bucket); err != nil {
			t.Errorf("remove test bucket: %v", err)
		}
	})
	putURL, err := client.PresignPut(ctx, objectKey, "video/mp4", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("isolated integration upload")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "video/mp4")
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned PUT status = %d", resp.StatusCode)
	}
	size, contentType, err := client.Stat(ctx, objectKey)
	if err != nil || size != int64(len(payload)) || contentType != "video/mp4" {
		t.Fatalf("stat size=%d type=%q err=%v", size, contentType, err)
	}
	getURL, err := client.PresignGet(ctx, objectKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	getResp, err := httpClient.Get(getURL)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	got, err := io.ReadAll(getResp.Body)
	if err != nil || getResp.StatusCode != http.StatusOK || !bytes.Equal(got, payload) {
		t.Fatalf("presigned GET status=%d bytes=%d err=%v", getResp.StatusCode, len(got), err)
	}
}
