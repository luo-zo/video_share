package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"video_share/internal/config"
)

func testClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	c, err := NewClient(&config.Config{
		MinIOEndpoint:  endpoint,
		MinIOAccessKey: "test-access",
		MinIOSecretKey: "test-secret-not-real",
		MinIOBucket:    "video-share",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPresignedURLsBindObjectExpiryAndUploadType(t *testing.T) {
	c := testClient(t, "127.0.0.1:9000")
	put, err := c.PresignPut(context.Background(), "videos/42/sample.mp4", "video/mp4", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(put)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "127.0.0.1:9000" || u.Path != "/video-share/videos/42/sample.mp4" || q.Get("X-Amz-Expires") != "600" {
		t.Fatal("upload URL has incorrect destination or expiry")
	}
	if !strings.Contains(q.Get("X-Amz-SignedHeaders"), "content-type") || q.Get("X-Amz-Signature") == "" {
		t.Fatal("upload URL does not bind the Content-Type header")
	}
	get, err := c.PresignGet(context.Background(), "videos/42/sample.mp4", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	play, _ := url.Parse(get)
	if play.Query().Get("X-Amz-Expires") != "3600" || play.Query().Get("response-content-type") != "video/mp4" {
		t.Fatal("playback URL has incorrect expiry or response media type")
	}
	if strings.Contains(put+get, "test-secret-not-real") {
		t.Fatal("secret key exposed in signed URL")
	}
}

func TestPresignedURLsUsePublicEndpoint(t *testing.T) {
	c, err := NewClient(&config.Config{
		MinIOEndpoint:       "minio:9000",
		MinIOPublicEndpoint: "127.0.0.1:9000",
		MinIOAccessKey:      "test-access",
		MinIOSecretKey:      "test-secret-not-real",
		MinIOBucket:         "video-share",
	})
	if err != nil {
		t.Fatal(err)
	}
	put, err := c.PresignPut(context.Background(), "videos/42/sample.mp4", "video/mp4", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(put)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "127.0.0.1:9000" {
		t.Fatalf("signed URL host = %q, want public endpoint", u.Host)
	}
}

func TestPresignRejectsInvalidArguments(t *testing.T) {
	c := testClient(t, "127.0.0.1:9000")
	if _, err := c.PresignPut(context.Background(), "x.mp4", "text/html", time.Minute); err == nil {
		t.Fatal("unexpected upload content type accepted")
	}
	for _, expiry := range []time.Duration{0, 8 * 24 * time.Hour} {
		if _, err := c.PresignPut(context.Background(), "x.mp4", "video/mp4", expiry); err == nil {
			t.Fatal("invalid upload expiry accepted")
		}
		if _, err := c.PresignGet(context.Background(), "x.mp4", expiry); err == nil {
			t.Fatal("invalid playback expiry accepted")
		}
	}
	if _, err := c.PresignGet(context.Background(), "", time.Minute); err == nil {
		t.Fatal("empty object key accepted")
	}
}

func TestEnsureBucketHandlesExistingCreationRaceAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		headStatus  int
		putCode     string
		wantCreates int32
		wantErr     bool
	}{
		{"existing", 200, "", 0, false},
		{"create", 404, "", 1, false},
		{"creation race", 404, "BucketAlreadyOwnedByYou", 1, false},
		{"denied", 403, "", 0, true},
		{"creation denied", 404, "AccessDenied", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var creates atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/video-share/" && r.URL.Path != "/video-share" {
					http.NotFound(w, r)
					return
				}
				if r.Method == http.MethodHead {
					w.WriteHeader(tc.headStatus)
					return
				}
				if r.Method != http.MethodPut {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				creates.Add(1)
				if tc.putCode != "" {
					w.Header().Set("Content-Type", "application/xml")
					status := http.StatusConflict
					if tc.putCode == "AccessDenied" {
						status = http.StatusForbidden
					}
					w.WriteHeader(status)
					fmt.Fprintf(w, "<Error><Code>%s</Code><Message>test response</Message></Error>", tc.putCode)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			c := testClient(t, strings.TrimPrefix(srv.URL, "http://"))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := c.EnsureBucket(ctx)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if creates.Load() != tc.wantCreates {
				t.Fatalf("create calls=%d want=%d", creates.Load(), tc.wantCreates)
			}
		})
	}
}

func TestStatReturnsStorageMetadataAndPropagatesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/video-share/existing.mp4" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", "1234")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"test-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := testClient(t, strings.TrimPrefix(srv.URL, "http://"))
	size, contentType, err := c.Stat(context.Background(), "existing.mp4")
	if err != nil || size != 1234 || contentType != "video/mp4" {
		t.Fatalf("Stat=(%d,%q,%v)", size, contentType, err)
	}
	if _, _, err := c.Stat(context.Background(), "missing.mp4"); err == nil {
		t.Fatal("missing object reported as success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Stat(ctx, "existing.mp4"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}
