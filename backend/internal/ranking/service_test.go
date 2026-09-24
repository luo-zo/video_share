package ranking

import (
	"context"
	"testing"
	"time"

	"video_share/internal/video"
)

type fakeRepository struct {
	candidates []Candidate
	listErr    error
}

func (f fakeRepository) Rebuild(context.Context, Window, time.Time) (time.Time, error) {
	return time.Time{}, nil
}

func (f fakeRepository) List(context.Context, Window, int, int, time.Time) ([]Candidate, int64, time.Time, error) {
	if f.listErr != nil {
		return nil, 0, time.Time{}, f.listErr
	}
	return f.candidates, int64(len(f.candidates)), time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC), nil
}

func TestServiceListKeepsSnapshotOrderAndRank(t *testing.T) {
	service := NewService(fakeRepository{candidates: []Candidate{
		{VideoID: 8, Score: 12, Video: video.Video{ID: 8, Title: "first"}},
		{VideoID: 3, Score: 9, Video: video.Video{ID: 3, Title: "second"}},
	}})
	result, err := service.List(context.Background(), WindowDay, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Items) != 2 || result.Items[0].Rank != 1 || result.Items[1].Rank != 2 || result.Items[1].VideoID != 3 {
		t.Fatalf("result=%+v", result)
	}
}

func TestServiceRejectsInvalidPageAndWindow(t *testing.T) {
	service := NewService(fakeRepository{})
	if _, err := service.List(context.Background(), WindowDay, 0, 20); err != ErrPagination {
		t.Fatalf("page error=%v", err)
	}
	if _, err := service.List(context.Background(), WindowDay, 1_000_001, 20); err != ErrPagination {
		t.Fatalf("huge page error=%v", err)
	}
	if _, err := service.List(context.Background(), Window("month"), 1, 20); err != ErrInvalidWindow {
		t.Fatalf("window error=%v", err)
	}
}
