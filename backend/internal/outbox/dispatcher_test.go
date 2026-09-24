package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	events        []Event
	published     []uint64
	failed        []uint64
	failureErrors []string
	retryAt       []time.Time
	claimLease    time.Duration
}

func (f *fakeRepository) ClaimDue(_ context.Context, _ int, lease time.Duration) ([]Event, error) {
	f.claimLease = lease
	return f.events, nil
}

func (f *fakeRepository) MarkPublished(_ context.Context, id uint64) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeRepository) MarkFailed(_ context.Context, id uint64, message string, retryAt time.Time) error {
	f.failed = append(f.failed, id)
	f.failureErrors = append(f.failureErrors, message)
	f.retryAt = append(f.retryAt, retryAt)
	return nil
}

type fakePublisher struct {
	err  error
	keys []string
}

func (f *fakePublisher) Publish(_ context.Context, _, key string, _ []byte) error {
	f.keys = append(f.keys, key)
	return f.err
}

type deadlinePublisher struct{}

func (deadlinePublisher) Publish(ctx context.Context, _, _ string, _ []byte) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("publish context has no deadline")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestDispatchOnceMarksPublishedEvents(t *testing.T) {
	repo := &fakeRepository{events: []Event{{ID: 7, Topic: "videos", EventKey: "42", Payload: []byte(`{"video_id":42}`)}}}
	publisher := &fakePublisher{}
	dispatcher := NewDispatcher(repo, publisher, time.Second, 10, nil)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.published) != 1 || repo.published[0] != 7 || len(repo.failed) != 0 {
		t.Fatalf("published=%v failed=%v", repo.published, repo.failed)
	}
}

func TestDispatchOnceSchedulesFailedPublish(t *testing.T) {
	repo := &fakeRepository{events: []Event{{ID: 8, Topic: "videos", EventKey: "43", Payload: []byte(`{}`)}}}
	publisher := &fakePublisher{err: errors.New("kafka unavailable")}
	dispatcher := NewDispatcher(repo, publisher, time.Second, 10, nil)

	if err := dispatcher.DispatchOnce(context.Background()); err == nil {
		t.Fatal("expected dispatch error")
	}
	if len(repo.failed) != 1 || repo.failed[0] != 8 || len(repo.published) != 0 {
		t.Fatalf("published=%v failed=%v", repo.published, repo.failed)
	}
}

func TestDispatchOnceBoundsPublishAndRecordsTimeout(t *testing.T) {
	repo := &fakeRepository{events: []Event{{ID: 9, Topic: "videos", EventKey: "44", Payload: []byte(`{}`)}}}
	dispatcher := NewDispatcherWithTimeout(repo, deadlinePublisher{}, time.Second, 20*time.Millisecond, 10, nil)

	started := time.Now()
	err := dispatcher.DispatchOnce(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DispatchOnce() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded publish took %s, want under 1s", elapsed)
	}
	if len(repo.failed) != 1 || repo.failed[0] != 9 || len(repo.published) != 0 {
		t.Fatalf("published=%v failed=%v", repo.published, repo.failed)
	}
	if len(repo.failureErrors) != 1 || !strings.Contains(repo.failureErrors[0], context.DeadlineExceeded.Error()) {
		t.Fatalf("failure errors = %q, want recorded publish timeout", repo.failureErrors)
	}
	if len(repo.retryAt) != 1 || !repo.retryAt[0].After(time.Now()) {
		t.Fatalf("retry times = %v, want a future retry", repo.retryAt)
	}
}

func TestDispatchOnceLeaseCoversSequentialPublishDeadlines(t *testing.T) {
	repo := &fakeRepository{}
	dispatcher := NewDispatcherWithTimeout(repo, &fakePublisher{}, time.Second, 20*time.Second, 3, nil)
	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	minimumLease := 3 * 20 * time.Second
	if repo.claimLease < minimumLease {
		t.Fatalf("claim lease = %s, want at least %s for a sequential batch", repo.claimLease, minimumLease)
	}
}
