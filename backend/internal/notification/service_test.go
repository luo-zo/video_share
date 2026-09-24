package notification

import (
	"context"
	"testing"
	"time"
)

type fakeRepository struct {
	items []Notification
}

func (f *fakeRepository) List(context.Context, uint64, int, int) ([]Notification, int64, int64, error) {
	return f.items, int64(len(f.items)), 1, nil
}
func (f *fakeRepository) MarkRead(context.Context, uint64, uint64) error     { return nil }
func (f *fakeRepository) MarkAllRead(context.Context, uint64) (int64, error) { return 1, nil }

func TestServiceProjectsStructuredNotificationsAndUnreadCount(t *testing.T) {
	actor, videoID := uint64(2), uint64(9)
	svc := NewService(&fakeRepository{items: []Notification{{ID: 1, RecipientID: 7, ActorID: &actor, Type: "comment", VideoID: &videoID, CreatedAt: time.Now()}}})
	got, err := svc.List(context.Background(), 7, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.UnreadCount != 1 || len(got.Items) != 1 || got.Items[0].VideoID == nil || *got.Items[0].VideoID != 9 {
		t.Fatalf("response = %+v", got)
	}
}
