package engagement

import (
	"context"
	"errors"
	"strings"
	"testing"

	"video_share/internal/video"
)

type mockRepository struct {
	setLikeFn       func(context.Context, uint64, uint64, bool) (*RelationState, error)
	setFavoriteFn   func(context.Context, uint64, uint64, bool) (*RelationState, error)
	createCommentFn func(context.Context, uint64, uint64, string) (*Comment, error)
	listCommentsFn  func(context.Context, uint64, int, int) ([]Comment, int64, error)
	deleteCommentFn func(context.Context, uint64, uint64) error
	recordWatchFn   func(context.Context, uint64, uint64, uint64, uint64) error
	listFavoritesFn func(context.Context, uint64, int, int) ([]video.Video, int64, error)
	listHistoryFn   func(context.Context, uint64, int, int) ([]video.Video, int64, error)
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

func (m *mockRepository) CreateComment(ctx context.Context, userID, videoID uint64, content string) (*Comment, error) {
	if m.createCommentFn == nil {
		return &Comment{}, nil
	}
	return m.createCommentFn(ctx, userID, videoID, content)
}

func (m *mockRepository) ListComments(ctx context.Context, videoID uint64, page, pageSize int) ([]Comment, int64, error) {
	if m.listCommentsFn == nil {
		return nil, 0, nil
	}
	return m.listCommentsFn(ctx, videoID, page, pageSize)
}

func (m *mockRepository) DeleteComment(ctx context.Context, userID, commentID uint64) error {
	if m.deleteCommentFn == nil {
		return nil
	}
	return m.deleteCommentFn(ctx, userID, commentID)
}

func (m *mockRepository) RecordWatch(ctx context.Context, userID, videoID, progressMS, durationMS uint64) error {
	if m.recordWatchFn == nil {
		return nil
	}
	return m.recordWatchFn(ctx, userID, videoID, progressMS, durationMS)
}

func (m *mockRepository) ListFavorites(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
	if m.listFavoritesFn == nil {
		return nil, 0, nil
	}
	return m.listFavoritesFn(ctx, userID, page, pageSize)
}

func (m *mockRepository) ListHistory(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
	if m.listHistoryFn == nil {
		return nil, 0, nil
	}
	return m.listHistoryFn(ctx, userID, page, pageSize)
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

func TestCreateCommentValidatesContentAndIdentity(t *testing.T) {
	cases := []struct {
		name    string
		userID  uint64
		videoID uint64
		content string
		want    error
	}{
		{name: "missing user", userID: 0, videoID: 9, content: "你好", want: ErrUnauthorized},
		{name: "missing video", userID: 7, videoID: 0, content: "你好", want: ErrVideoNotFound},
		{name: "blank content", userID: 7, videoID: 9, content: "   ", want: ErrContentInvalid},
		{name: "content too long", userID: 7, videoID: 9, content: strings.Repeat("字", 501), want: ErrContentInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.createCommentFn = func(context.Context, uint64, uint64, string) (*Comment, error) {
				t.Fatal("rejected request must not reach the repository")
				return nil, nil
			}
			svc := NewService(repo)

			if _, err := svc.CreateComment(context.Background(), tc.userID, tc.videoID, tc.content); !errors.Is(err, tc.want) {
				t.Fatalf("CreateComment error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateCommentTrimsContentAndProjectsAuthor(t *testing.T) {
	repo := &mockRepository{}
	repo.createCommentFn = func(_ context.Context, userID, videoID uint64, content string) (*Comment, error) {
		if userID != 7 || videoID != 9 || content != "你好" {
			t.Fatalf("CreateComment args = %d/%d/%q", userID, videoID, content)
		}
		return &Comment{
			ID: 3, VideoID: 9, UserID: 7, Content: content,
			Author: video.Author{ID: 7, Username: "bob", Nickname: "鲍勃"},
		}, nil
	}
	svc := NewService(repo)

	got, err := svc.CreateComment(context.Background(), 7, 9, "  你好  ")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if got.ID != 3 || got.Content != "你好" || got.Author == nil || got.Author.Nickname != "鲍勃" {
		t.Fatalf("CommentResponse = %+v", got)
	}
}

func TestDeleteCommentValidatesIdentity(t *testing.T) {
	cases := []struct {
		name      string
		userID    uint64
		commentID uint64
		want      error
	}{
		{name: "missing user", userID: 0, commentID: 5, want: ErrUnauthorized},
		{name: "missing comment", userID: 7, commentID: 0, want: ErrCommentNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.deleteCommentFn = func(context.Context, uint64, uint64) error {
				t.Fatal("rejected request must not reach the repository")
				return nil
			}
			svc := NewService(repo)

			if err := svc.DeleteComment(context.Background(), tc.userID, tc.commentID); !errors.Is(err, tc.want) {
				t.Fatalf("DeleteComment error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRecordWatchValidatesProgressAndIdentity(t *testing.T) {
	cases := []struct {
		name       string
		userID     uint64
		videoID    uint64
		progressMS uint64
		durationMS uint64
		want       error
	}{
		{name: "missing user", userID: 0, videoID: 9, progressMS: 1, durationMS: 10, want: ErrUnauthorized},
		{name: "missing video", userID: 7, videoID: 0, progressMS: 1, durationMS: 10, want: ErrVideoNotFound},
		{name: "progress beyond duration", userID: 7, videoID: 9, progressMS: 11, durationMS: 10, want: ErrProgressInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.recordWatchFn = func(context.Context, uint64, uint64, uint64, uint64) error {
				t.Fatal("rejected request must not reach the repository")
				return nil
			}
			svc := NewService(repo)

			if err := svc.RecordWatch(context.Background(), tc.userID, tc.videoID, tc.progressMS, tc.durationMS); !errors.Is(err, tc.want) {
				t.Fatalf("RecordWatch error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRecordWatchAllowsProgressWhenDurationIsUnknown(t *testing.T) {
	repo := &mockRepository{}
	repo.recordWatchFn = func(_ context.Context, userID, videoID, progressMS, durationMS uint64) error {
		if userID != 7 || videoID != 9 || progressMS != 50 || durationMS != 0 {
			t.Fatalf("RecordWatch args = %d/%d/%d/%d", userID, videoID, progressMS, durationMS)
		}
		return nil
	}
	svc := NewService(repo)

	if err := svc.RecordWatch(context.Background(), 7, 9, 50, 0); err != nil {
		t.Fatalf("RecordWatch: %v", err)
	}
}

func TestListCommentsValidatesPaginationAndProjectsAuthor(t *testing.T) {
	repo := &mockRepository{}
	repo.listCommentsFn = func(_ context.Context, videoID uint64, page, pageSize int) ([]Comment, int64, error) {
		if videoID != 9 || page != 2 || pageSize != 5 {
			t.Fatalf("ListComments args = %d/%d/%d", videoID, page, pageSize)
		}
		return []Comment{{
			ID: 2, VideoID: 9, UserID: 7, Content: "不错",
			Author: video.Author{ID: 7, Username: "alice", Nickname: "爱丽丝"},
		}}, 1, nil
	}
	svc := NewService(repo)

	got, err := svc.ListComments(context.Background(), 9, 2, 5)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Page != 2 || got.PageSize != 5 {
		t.Fatalf("CommentListResponse = %+v", got)
	}
	if got.Items[0].Author == nil || got.Items[0].Author.Nickname != "爱丽丝" || got.Items[0].Content != "不错" {
		t.Fatalf("item = %+v", got.Items[0])
	}

	for _, tc := range []struct{ page, pageSize int }{{0, 5}, {1, 0}, {1, 51}} {
		if _, err := svc.ListComments(context.Background(), 9, tc.page, tc.pageSize); !errors.Is(err, ErrPaginationInvalid) {
			t.Fatalf("ListComments page=%d size=%d error = %v, want ErrPaginationInvalid", tc.page, tc.pageSize, err)
		}
	}
}

func TestPersonalListsValidatePaginationAndMapVideos(t *testing.T) {
	repo := &mockRepository{}
	repo.listFavoritesFn = func(_ context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
		if userID != 7 || page != 1 || pageSize != 12 {
			t.Fatalf("ListFavorites args = %d/%d/%d", userID, page, pageSize)
		}
		return []video.Video{{
			ID: 4, UserID: 8, Title: "收藏的视频",
			Author: video.Author{ID: 8, Username: "bob", Nickname: "鲍勃"},
			Stats:  video.Stats{LikeCount: 2},
		}}, 1, nil
	}
	repo.listHistoryFn = func(_ context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
		if userID != 7 || page != 2 || pageSize != 3 {
			t.Fatalf("ListHistory args = %d/%d/%d", userID, page, pageSize)
		}
		return []video.Video{{
			ID: 5, UserID: 8, Title: "看过的视频",
			Author: video.Author{ID: 8, Username: "bob", Nickname: "鲍勃"},
			Stats:  video.Stats{ViewCount: 9},
		}}, 1, nil
	}
	svc := NewService(repo)

	favorites, err := svc.ListFavorites(context.Background(), 7, 1, 12)
	if err != nil || len(favorites.Items) != 1 || favorites.Items[0].Title != "收藏的视频" || favorites.Items[0].Stats.LikeCount != 2 {
		t.Fatalf("ListFavorites = (%+v, %v)", favorites, err)
	}
	if favorites.Items[0].Author == nil || favorites.Items[0].Author.Username != "bob" {
		t.Fatalf("favorite author = %+v", favorites.Items[0].Author)
	}

	history, err := svc.ListHistory(context.Background(), 7, 2, 3)
	if err != nil || history.Page != 2 || history.PageSize != 3 || len(history.Items) != 1 || history.Items[0].Stats.ViewCount != 9 {
		t.Fatalf("ListHistory = (%+v, %v)", history, err)
	}

	if _, err := svc.ListFavorites(context.Background(), 0, 1, 12); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ListFavorites without user error = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ListHistory(context.Background(), 7, 0, 12); !errors.Is(err, ErrPaginationInvalid) {
		t.Fatalf("ListHistory bad page error = %v, want ErrPaginationInvalid", err)
	}
}
