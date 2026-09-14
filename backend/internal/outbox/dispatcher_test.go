package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	events    []Event
	published []uint64
	failed    []uint64
}

func (f *fakeRepository) ClaimDue(context.Context, int, time.Duration) ([]Event, error) {
	return f.events, nil
}

func (f *fakeRepository) MarkPublished(_ context.Context, id uint64) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeRepository) MarkFailed(_ context.Context, id uint64, _ string, _ time.Time) error {
	f.failed = append(f.failed, id)
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
