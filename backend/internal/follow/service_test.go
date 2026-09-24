package follow

import (
	"context"
	"errors"
	"testing"
)

type mockRepository struct {
	setFollowFn     func(context.Context, uint64, uint64, bool) (*FollowState, error)
	listFollowsFn   func(context.Context, uint64, int, int) ([]FollowedUser, int64, error)
	listFollowersFn func(context.Context, uint64, int, int) ([]FollowedUser, int64, error)
}

func (m *mockRepository) SetFollow(ctx context.Context, followerID, followeeID uint64, active bool) (*FollowState, error) {
	if m.setFollowFn == nil {
		return &FollowState{}, nil
	}
	return m.setFollowFn(ctx, followerID, followeeID, active)
}

func (m *mockRepository) ListFollows(ctx context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
	if m.listFollowsFn == nil {
		return nil, 0, nil
	}
	return m.listFollowsFn(ctx, followerID, page, pageSize)
}

func (m *mockRepository) ListFollowing(ctx context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
	return m.ListFollows(ctx, followerID, page, pageSize)
}

func (m *mockRepository) ListFollowers(ctx context.Context, followeeID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
	if m.listFollowersFn == nil {
		return nil, 0, nil
	}
	return m.listFollowersFn(ctx, followeeID, page, pageSize)
}

func TestFollowRejectsMissingUserTargetAndSelfFollow(t *testing.T) {
	cases := []struct {
		name       string
		followerID uint64
		followeeID uint64
		wantErr    error
	}{
		{"missing viewer", 0, 5, ErrUnauthorized},
		{"missing target", 5, 0, ErrUserNotFound},
		{"self follow", 3, 3, ErrSelfFollow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			repo := &mockRepository{setFollowFn: func(context.Context, uint64, uint64, bool) (*FollowState, error) {
				called = true
				return &FollowState{}, nil
			}}
			svc := NewService(repo)

			if _, err := svc.Follow(context.Background(), tc.followerID, tc.followeeID); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Follow err = %v, want %v", err, tc.wantErr)
			}
			if _, err := svc.Unfollow(context.Background(), tc.followerID, tc.followeeID); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Unfollow err = %v, want %v", err, tc.wantErr)
			}
			if called {
				t.Fatal("repository must not be called when validation fails")
			}
		})
	}
}

func TestFollowPassesIdentityAndStateToRepository(t *testing.T) {
	var seenFollower, seenFollowee uint64
	var seenActive []bool
	repo := &mockRepository{setFollowFn: func(_ context.Context, followerID, followeeID uint64, active bool) (*FollowState, error) {
		seenFollower, seenFollowee = followerID, followeeID
		seenActive = append(seenActive, active)
		return &FollowState{Following: active}, nil
	}}
	svc := NewService(repo)

	followed, err := svc.Follow(context.Background(), 7, 9)
	if err != nil || !followed.Following {
		t.Fatalf("Follow = %+v, %v", followed, err)
	}
	unfollowed, err := svc.Unfollow(context.Background(), 7, 9)
	if err != nil || unfollowed.Following {
		t.Fatalf("Unfollow = %+v, %v", unfollowed, err)
	}
	if seenFollower != 7 || seenFollowee != 9 {
		t.Fatalf("repository saw %d -> %d", seenFollower, seenFollowee)
	}
	if len(seenActive) != 2 || !seenActive[0] || seenActive[1] {
		t.Fatalf("active flags = %v", seenActive)
	}
}

func TestListFollowsValidatesPaginationAndViewer(t *testing.T) {
	called := false
	repo := &mockRepository{listFollowsFn: func(context.Context, uint64, int, int) ([]FollowedUser, int64, error) {
		called = true
		return nil, 0, nil
	}}
	svc := NewService(repo)

	for _, tc := range []struct{ page, pageSize int }{{0, defaultPageSize}, {1, 0}, {1, maxPageSize + 1}} {
		if _, err := svc.ListFollows(context.Background(), 7, tc.page, tc.pageSize); !errors.Is(err, ErrPaginationInvalid) {
			t.Fatalf("ListFollows(%d,%d) err = %v, want pagination error", tc.page, tc.pageSize, err)
		}
	}
	if _, err := svc.ListFollows(context.Background(), 0, 1, defaultPageSize); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing viewer err = %v, want unauthorized", err)
	}
	if called {
		t.Fatal("repository must not be called when validation fails")
	}
}

func TestListFollowsMapsUsersAndTotal(t *testing.T) {
	repo := &mockRepository{listFollowsFn: func(_ context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
		if followerID != 7 || page != 2 || pageSize != 20 {
			t.Fatalf("repository args = %d/%d/%d", followerID, page, pageSize)
		}
		return []FollowedUser{
			{ID: 11, Username: "bob", Nickname: "Bob"},
			{ID: 12, Username: "carol", Nickname: "Carol"},
		}, 5, nil
	}}
	result, err := NewService(repo).ListFollows(context.Background(), 7, 2, 20)
	if err != nil {
		t.Fatalf("ListFollows: %v", err)
	}
	if result.Total != 5 || result.Page != 2 || result.PageSize != 20 || len(result.Items) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].ID != 11 || result.Items[0].Username != "bob" || result.Items[1].Nickname != "Carol" {
		t.Fatalf("items = %+v", result.Items)
	}
}
