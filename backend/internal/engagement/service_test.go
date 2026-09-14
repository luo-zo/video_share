package engagement

import (
	"context"
	"errors"
	"testing"
)

type mockRepository struct {
	setLikeFn     func(context.Context, uint64, uint64, bool) (*RelationState, error)
	setFavoriteFn func(context.Context, uint64, uint64, bool) (*RelationState, error)
}

func (m *mockRepository) SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	if m.setLikeFn == nil {
		return &RelationState{}, nil
	}
	return m.setLikeFn(ctx, userID, videoID, active)
}

func (m *mockRepository) SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	if m.setFavoriteFn == nil {
		return &RelationState{}, nil
	}
	return m.setFavoriteFn(ctx, userID, videoID, active)
}

func TestLikeAndFavoriteRejectMissingUserOrVideo(t *testing.T) {
	cases := []struct {
		name    string
		userID  uint64
		videoID uint64
		active  bool
		want    error
	}{
		{name: "like without user", userID: 0, videoID: 9, active: true, want: ErrUnauthorized},
		{name: "unlike without user", userID: 0, videoID: 9, active: false, want: ErrUnauthorized},
		{name: "like without video", userID: 7, videoID: 0, active: true, want: ErrVideoNotFound},
		{name: "favorite without video", userID: 7, videoID: 0, active: false, want: ErrVideoNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.setLikeFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) {
				t.Fatal("rejected request must not reach the repository")
				return nil, nil
			}
			repo.setFavoriteFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) {
				t.Fatal("rejected request must not reach the repository")
				return nil, nil
			}
			svc := NewService(repo)

			if _, err := svc.SetLike(context.Background(), tc.userID, tc.videoID, tc.active); !errors.Is(err, tc.want) {
				t.Fatalf("SetLike error = %v, want %v", err, tc.want)
			}
			if _, err := svc.SetFavorite(context.Background(), tc.userID, tc.videoID, tc.active); !errors.Is(err, tc.want) {
				t.Fatalf("SetFavorite error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestLikeAndFavoriteReturnTheFinalRepositoryState(t *testing.T) {
	repo := &mockRepository{}
	repo.setLikeFn = func(_ context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
		if userID != 7 || videoID != 9 || !active {
			t.Fatalf("SetLike args = %d/%d/%v", userID, videoID, active)
		}
		return &RelationState{Active: true, Count: 4}, nil
	}
	repo.setFavoriteFn = func(_ context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
		if userID != 7 || videoID != 9 || active {
			t.Fatalf("SetFavorite args = %d/%d/%v", userID, videoID, active)
		}
		return &RelationState{Active: false, Count: 2}, nil
	}
	svc := NewService(repo)

	like, err := svc.SetLike(context.Background(), 7, 9, true)
	if err != nil || !like.Active || like.Count != 4 {
		t.Fatalf("SetLike = (%+v, %v)", like, err)
	}
	favorite, err := svc.SetFavorite(context.Background(), 7, 9, false)
	if err != nil || favorite.Active || favorite.Count != 2 {
		t.Fatalf("SetFavorite = (%+v, %v)", favorite, err)
	}
}

func TestLikeAndFavoriteSurfaceUnavailableVideo(t *testing.T) {
	repo := &mockRepository{}
	repo.setLikeFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) {
		return nil, ErrVideoNotFound
	}
	repo.setFavoriteFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) {
		return nil, ErrVideoNotFound
	}
	svc := NewService(repo)

	if _, err := svc.SetLike(context.Background(), 7, 9, true); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("SetLike error = %v, want ErrVideoNotFound", err)
	}
	if _, err := svc.SetFavorite(context.Background(), 7, 9, true); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("SetFavorite error = %v, want ErrVideoNotFound", err)
	}
}
