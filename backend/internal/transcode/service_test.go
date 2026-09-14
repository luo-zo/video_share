package transcode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"video_share/internal/messaging"
)

type fakeJobs struct {
	claimErr  error
	job       *Job
	progress  []uint8
	succeeded *Output
	failed    bool
}

func (f *fakeJobs) Claim(context.Context, string, uint64, string, time.Duration) (*Job, error) {
	return f.job, f.claimErr
}
func (f *fakeJobs) UpdateProgress(_ context.Context, _ *Job, progress uint8) error {
	f.progress = append(f.progress, progress)
	return nil
}
func (f *fakeJobs) Succeed(_ context.Context, _ *Job, output Output) error {
	f.succeeded = &output
	return nil
}
func (f *fakeJobs) Fail(context.Context, *Job, messaging.TranscodeRequested, error, time.Duration) (bool, error) {
	f.failed = true
	return false, nil
}

type fakeMediaStore struct{ uploads []string }

func (f *fakeMediaStore) Download(_ context.Context, _ string, destination string) error {
	return os.WriteFile(destination, []byte("source"), 0o600)
}
func (f *fakeMediaStore) UploadFile(_ context.Context, key, _, _ string) error {
	f.uploads = append(f.uploads, key)
	return nil
}

type fakeProber struct{ info MediaInfo }

func (f fakeProber) Probe(context.Context, string) (MediaInfo, error) { return f.info, nil }

type fakeRunner struct{ err error }

func (f fakeRunner) Process(_ context.Context, _, outputDir string, _ MediaInfo, _ []Profile) error {
	if f.err != nil {
		return f.err
	}
	for name, content := range map[string]string{
		"cover.jpg":             "cover",
		"master.m3u8":           "master",
		"360p/index.m3u8":       "variant",
		"360p/segment-00000.ts": "segment",
	} {
		path := filepath.Join(outputDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func TestServiceProcessesAndPublishesOutputs(t *testing.T) {
	jobs := &fakeJobs{job: &Job{ID: 1, JobID: "job-1", VideoID: 42, Attempts: 1, MaxAttempts: 3}}
	store := &fakeMediaStore{}
	service := NewService(jobs, store, fakeProber{MediaInfo{DurationMS: 1234, Width: 640, Height: 360}}, fakeRunner{}, "worker-1", t.TempDir(), 5*time.Minute, time.Second, nil)
	event := messaging.TranscodeRequested{JobID: "job-1", VideoID: 42, UserID: 7, SourceObjectKey: "videos/7/42/source.mp4", SchemaVersion: 1}

	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if jobs.succeeded == nil || jobs.succeeded.HLSMasterKey != "videos/7/42/processed/v1/master.m3u8" || jobs.succeeded.CoverObjectKey != "videos/7/42/cover/v1/cover.jpg" {
		t.Fatalf("output = %+v", jobs.succeeded)
	}
	if len(store.uploads) != 4 || jobs.failed {
		t.Fatalf("uploads=%v failed=%v", store.uploads, jobs.failed)
	}
}

func TestServiceRecordsProcessingFailureForRetry(t *testing.T) {
	jobs := &fakeJobs{job: &Job{ID: 1, JobID: "job-1", VideoID: 42, Attempts: 1, MaxAttempts: 3}}
	service := NewService(jobs, &fakeMediaStore{}, fakeProber{MediaInfo{DurationMS: 1000, Width: 640, Height: 360}}, fakeRunner{err: errors.New("ffmpeg failed")}, "worker-1", t.TempDir(), time.Minute, time.Second, nil)
	if err := service.Process(context.Background(), messaging.TranscodeRequested{JobID: "job-1", VideoID: 42, UserID: 7, SourceObjectKey: "source", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if !jobs.failed || jobs.succeeded != nil {
		t.Fatalf("failed=%v succeeded=%+v", jobs.failed, jobs.succeeded)
	}
}

func TestServiceIgnoresAlreadyProcessedJob(t *testing.T) {
	jobs := &fakeJobs{claimErr: ErrJobAlreadyProcessed}
	service := NewService(jobs, &fakeMediaStore{}, fakeProber{}, fakeRunner{}, "worker-1", t.TempDir(), time.Minute, time.Second, nil)
	if err := service.Process(context.Background(), messaging.TranscodeRequested{JobID: "job-1", VideoID: 42, SourceObjectKey: "source", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
}
