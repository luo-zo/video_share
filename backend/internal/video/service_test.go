package video

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testMaxFileSize int64 = 500 * 1024 * 1024

var (
	testUploadExpiry = 10 * time.Minute
	testPlayExpiry   = time.Hour
)

type mockRepository struct {
	createFn          func(context.Context, *Video) error
	updateObjectKeyFn func(context.Context, uint64, string) error
	findByIDFn        func(context.Context, uint64) (*Video, error)
	beginProcessingFn func(context.Context, *Video) error
	markReadyFn       func(context.Context, uint64) error
	listReadyFn       func(context.Context, int, int) ([]Video, int64, error)
	findReadyByIDFn   func(context.Context, uint64) (*Video, error)
	listPublicFn      func(context.Context, ListQuery) ([]Video, int64, error)
	findPublicByIDFn  func(context.Context, uint64) (*Video, error)
	viewerStateFn     func(context.Context, uint64, uint64, uint64) (*ViewerState, error)
	listByUserFn      func(context.Context, uint64, int, int) ([]Video, int64, error)
	updateOwnedFn     func(context.Context, uint64, uint64, VideoPatch) error
	deleteOwnedFn     func(context.Context, uint64, uint64) error
}

func (m *mockRepository) UpdateOwned(ctx context.Context, userID, videoID uint64, patch VideoPatch) error {
	if m.updateOwnedFn == nil {
		return nil
	}
	return m.updateOwnedFn(ctx, userID, videoID, patch)
}

func (m *mockRepository) DeleteOwned(ctx context.Context, userID, videoID uint64) error {
	if m.deleteOwnedFn == nil {
		return nil
	}
	return m.deleteOwnedFn(ctx, userID, videoID)
}

func strPtr(value string) *string { return &value }

func (m *mockRepository) Create(ctx context.Context, v *Video) error {
	if m.createFn == nil {
		return nil
	}
	return m.createFn(ctx, v)
}

func (m *mockRepository) UpdateObjectKey(ctx context.Context, id uint64, key string) error {
	if m.updateObjectKeyFn == nil {
		return nil
	}
	return m.updateObjectKeyFn(ctx, id, key)
}

func (m *mockRepository) FindByID(ctx context.Context, id uint64) (*Video, error) {
	if m.findByIDFn == nil {
		return nil, ErrNotFound
	}
	return m.findByIDFn(ctx, id)
}

func (m *mockRepository) BeginProcessing(ctx context.Context, video *Video) error {
	if m.beginProcessingFn == nil {
		return nil
	}
	return m.beginProcessingFn(ctx, video)
}

func (m *mockRepository) MarkReady(ctx context.Context, id uint64) error {
	if m.markReadyFn == nil {
		return nil
	}
	return m.markReadyFn(ctx, id)
}

func (m *mockRepository) ListReady(ctx context.Context, page, pageSize int) ([]Video, int64, error) {
	if m.listReadyFn == nil {
		return []Video{}, 0, nil
	}
	return m.listReadyFn(ctx, page, pageSize)
}

func (m *mockRepository) FindReadyByID(ctx context.Context, id uint64) (*Video, error) {
	if m.findReadyByIDFn == nil {
		return nil, ErrNotFound
	}
	return m.findReadyByIDFn(ctx, id)
}

func (m *mockRepository) ListByUser(ctx context.Context, userID uint64, page, pageSize int) ([]Video, int64, error) {
	if m.listByUserFn == nil {
		return []Video{}, 0, nil
	}
	return m.listByUserFn(ctx, userID, page, pageSize)
}

type mockObjectStore struct {
	presignPutFn func(context.Context, string, string, time.Duration) (string, error)
	presignGetFn func(context.Context, string, time.Duration) (string, error)
	statFn       func(context.Context, string) (int64, string, error)
	readFn       func(context.Context, string, int64) ([]byte, error)
}

func (m *mockObjectStore) PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	if m.presignPutFn == nil {
		return "https://storage.example/upload", nil
	}
	return m.presignPutFn(ctx, key, contentType, expiry)
}

func (m *mockObjectStore) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if m.presignGetFn == nil {
		return "https://storage.example/play", nil
	}
	return m.presignGetFn(ctx, key, expiry)
}

func (m *mockObjectStore) PresignGetAs(ctx context.Context, key, _ string, expiry time.Duration) (string, error) {
	return m.PresignGet(ctx, key, expiry)
}

func (m *mockObjectStore) Read(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	if m.readFn == nil {
		return nil, errors.New("unexpected read")
	}
	return m.readFn(ctx, key, maxBytes)
}

func (m *mockObjectStore) Stat(ctx context.Context, key string) (int64, string, error) {
	if m.statFn == nil {
		return 0, "", errors.New("unexpected stat")
	}
	return m.statFn(ctx, key)
}

func newTestService(repo Repository, store ObjectStore) *Service {
	return NewService(repo, store, testMaxFileSize, testUploadExpiry, testPlayExpiry)
}

func TestCreateValidatesPersistsAndPresigns(t *testing.T) {
	repo := &mockRepository{}
	store := &mockObjectStore{}
	createdAt := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	var persisted *Video
	var updatedKey string

	repo.createFn = func(_ context.Context, v *Video) error {
		v.ID = 42
		v.CreatedAt = createdAt
		v.UpdatedAt = createdAt
		copy := *v
		persisted = &copy
		return nil
	}
	repo.updateObjectKeyFn = func(_ context.Context, id uint64, key string) error {
		if id != 42 {
			t.Fatalf("UpdateObjectKey id = %d, want 42", id)
		}
		updatedKey = key
		return nil
	}
	store.presignPutFn = func(_ context.Context, key, contentType string, expiry time.Duration) (string, error) {
		if key != "videos/7/42/source.mp4" {
			t.Fatalf("PresignPut key = %q", key)
		}
		if contentType != "video/mp4" || expiry != testUploadExpiry {
			t.Fatalf("PresignPut metadata = (%q, %v)", contentType, expiry)
		}
		return "https://storage.example/upload/42", nil
	}

	got, err := newTestService(repo, store).Create(context.Background(), 7, CreateRequest{
		Title:       "  第一条视频  ",
		Description: "  一段简介  ",
		FileName:    "clip.MP4",
		ContentType: "video/mp4",
		FileSize:    1024,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if persisted == nil || persisted.Title != "第一条视频" || persisted.Description != "一段简介" {
		t.Fatalf("persisted = %+v", persisted)
	}
	if persisted.UserID != 7 || persisted.Status != StatusUploading || !strings.HasPrefix(persisted.ObjectKey, "pending/7/") {
		t.Fatalf("unexpected initial video: %+v", persisted)
	}
	if updatedKey != "videos/7/42/source.mp4" {
		t.Fatalf("updated key = %q", updatedKey)
	}
	if got.ID != 42 || got.Status != "uploading" || got.UploadURL != "https://storage.example/upload/42" {
		t.Fatalf("response = %+v", got)
	}
	if got.UploadExpiresIn != int64(testUploadExpiry.Seconds()) {
		t.Fatalf("upload_expires_in = %d", got.UploadExpiresIn)
	}
}

func TestCreateRejectsInvalidInputBeforePersistence(t *testing.T) {
	cases := []struct {
		name string
		req  CreateRequest
		err  error
	}{
		{"empty title", CreateRequest{FileName: "a.mp4", ContentType: "video/mp4", FileSize: 1}, ErrTitleInvalid},
		{"long title", CreateRequest{Title: strings.Repeat("题", 101), FileName: "a.mp4", ContentType: "video/mp4", FileSize: 1}, ErrTitleInvalid},
		{"long description", CreateRequest{Title: "ok", Description: strings.Repeat("介", 2001), FileName: "a.mp4", ContentType: "video/mp4", FileSize: 1}, ErrDescriptionInvalid},
		{"empty filename", CreateRequest{Title: "ok", ContentType: "video/mp4", FileSize: 1}, ErrFileNameInvalid},
		{"wrong extension", CreateRequest{Title: "ok", FileName: "a.mov", ContentType: "video/mp4", FileSize: 1}, ErrFileNameInvalid},
		{"extension only", CreateRequest{Title: "ok", FileName: ".mp4", ContentType: "video/mp4", FileSize: 1}, ErrFileNameInvalid},
		{"wrong content type", CreateRequest{Title: "ok", FileName: "a.mp4", ContentType: "application/octet-stream", FileSize: 1}, ErrContentTypeInvalid},
		{"zero size", CreateRequest{Title: "ok", FileName: "a.mp4", ContentType: "video/mp4"}, ErrFileSizeInvalid},
		{"too large", CreateRequest{Title: "ok", FileName: "a.mp4", ContentType: "video/mp4", FileSize: testMaxFileSize + 1}, ErrFileSizeInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{createFn: func(context.Context, *Video) error {
				t.Fatal("Create must not persist invalid input")
				return nil
			}}
			_, err := newTestService(repo, &mockObjectStore{}).Create(context.Background(), 7, tc.req)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestCreateRejectsMissingUser(t *testing.T) {
	_, err := newTestService(&mockRepository{}, &mockObjectStore{}).Create(context.Background(), 0, CreateRequest{
		Title: "ok", FileName: "a.mp4", ContentType: "video/mp4", FileSize: 1,
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
}

func TestCompleteVerifiesOwnerObjectAndQueuesProcessing(t *testing.T) {
	v := &Video{ID: 42, UserID: 7, ObjectKey: "videos/7/42/source.mp4", Status: StatusUploading, FileSize: 1234, ContentType: "video/mp4"}
	repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) { copy := *v; return &copy, nil }}
	queued := false
	repo.beginProcessingFn = func(_ context.Context, got *Video) error {
		queued = true
		if got.ID != 42 || got.ObjectKey != v.ObjectKey {
			t.Fatalf("BeginProcessing video = %+v", got)
		}
		return nil
	}
	store := &mockObjectStore{statFn: func(_ context.Context, key string) (int64, string, error) {
		if key != v.ObjectKey {
			t.Fatalf("Stat key = %q", key)
		}
		return 1234, "video/mp4", nil
	}}

	got, err := newTestService(repo, store).Complete(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !queued || got.Status != "processing" {
		t.Fatalf("queued=%v response=%+v", queued, got)
	}
}

func TestCompleteIsIdempotentWhenProcessing(t *testing.T) {
	repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) {
		return &Video{ID: 42, UserID: 7, Status: StatusProcessing}, nil
	}}
	store := &mockObjectStore{statFn: func(context.Context, string) (int64, string, error) {
		t.Fatal("idempotent complete must not stat again")
		return 0, "", nil
	}}

	got, err := newTestService(repo, store).Complete(context.Background(), 7, 42)
	if err != nil || got.Status != "processing" {
		t.Fatalf("Complete = (%+v, %v)", got, err)
	}
}

func TestCompleteIsIdempotentWhenReady(t *testing.T) {
	repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) {
		return &Video{ID: 42, UserID: 7, Status: StatusReady}, nil
	}}
	store := &mockObjectStore{statFn: func(context.Context, string) (int64, string, error) {
		t.Fatal("idempotent complete must not stat again")
		return 0, "", nil
	}}

	got, err := newTestService(repo, store).Complete(context.Background(), 7, 42)
	if err != nil || got.Status != "ready" {
		t.Fatalf("Complete = (%+v, %v)", got, err)
	}
}

func TestCompleteRejectsNonOwnerBeforeStat(t *testing.T) {
	repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) {
		return &Video{ID: 42, UserID: 8, Status: StatusUploading}, nil
	}}
	store := &mockObjectStore{statFn: func(context.Context, string) (int64, string, error) {
		t.Fatal("non-owner complete must not inspect the object")
		return 0, "", nil
	}}
	_, err := newTestService(repo, store).Complete(context.Background(), 7, 42)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestCompleteRejectsMissingOrMismatchedObject(t *testing.T) {
	base := &Video{ID: 42, UserID: 7, ObjectKey: "videos/7/42/source.mp4", Status: StatusUploading, FileSize: 1234, ContentType: "video/mp4"}
	cases := []struct {
		name string
		stat func(context.Context, string) (int64, string, error)
		err  error
	}{
		{"missing", func(context.Context, string) (int64, string, error) { return 0, "", errors.New("not found") }, ErrUploadIncomplete},
		{"size", func(context.Context, string) (int64, string, error) { return 1235, "video/mp4", nil }, ErrUploadMismatch},
		{"type", func(context.Context, string) (int64, string, error) { return 1234, "video/quicktime", nil }, ErrUploadMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) { copy := *base; return &copy, nil }}
			repo.markReadyFn = func(context.Context, uint64) error {
				t.Fatal("invalid object must not become ready")
				return nil
			}
			_, err := newTestService(repo, &mockObjectStore{statFn: tc.stat}).Complete(context.Background(), 7, 42)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestCompleteRejectsFailedAndDeletedStates(t *testing.T) {
	for _, tc := range []struct {
		status Status
		err    error
	}{{StatusFailed, ErrStateConflict}, {StatusDeleted, ErrNotFound}} {
		repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) {
			return &Video{ID: 42, UserID: 7, Status: tc.status}, nil
		}}
		_, err := newTestService(repo, &mockObjectStore{}).Complete(context.Background(), 7, 42)
		if !errors.Is(err, tc.err) {
			t.Fatalf("status %d: error = %v, want %v", tc.status, err, tc.err)
		}
	}
}

func TestListDetailAndMine(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	item := Video{
		ID: 42, UserID: 7, Title: "hello", ObjectKey: "videos/7/42/source.mp4",
		Status: StatusReady, FileSize: 99, ContentType: "video/mp4", CreatedAt: now,
		Author: Author{ID: 7, Username: "alice", Nickname: "爱丽丝"},
	}
	repo := &mockRepository{}
	repo.listReadyFn = func(_ context.Context, page, pageSize int) ([]Video, int64, error) {
		if page != 2 || pageSize != 12 {
			t.Fatalf("ListReady pagination = %d/%d", page, pageSize)
		}
		return []Video{item}, 25, nil
	}
	repo.findReadyByIDFn = func(_ context.Context, id uint64) (*Video, error) {
		if id != 42 {
			t.Fatalf("detail id = %d", id)
		}
		copy := item
		return &copy, nil
	}
	repo.listByUserFn = func(_ context.Context, userID uint64, page, pageSize int) ([]Video, int64, error) {
		if userID != 7 || page != 1 || pageSize != 12 {
			t.Fatalf("ListByUser args = %d/%d/%d", userID, page, pageSize)
		}
		uploading := item
		uploading.Status = StatusUploading
		return []Video{uploading}, 1, nil
	}
	store := &mockObjectStore{presignGetFn: func(_ context.Context, key string, expiry time.Duration) (string, error) {
		if key != item.ObjectKey || expiry != testPlayExpiry {
			t.Fatalf("PresignGet args = %q/%v", key, expiry)
		}
		return "https://storage.example/play/42", nil
	}}
	svc := newTestService(repo, store)

	list, err := svc.List(context.Background(), 2, 12)
	if err != nil || list.Total != 25 || len(list.Items) != 1 || list.Items[0].Author == nil {
		t.Fatalf("List = (%+v, %v)", list, err)
	}
	detail, err := svc.Detail(context.Background(), 42)
	if err != nil || detail.PlayURL == "" || detail.Author == nil || detail.Author.Username != "alice" {
		t.Fatalf("Detail = (%+v, %v)", detail, err)
	}
	mine, err := svc.Mine(context.Background(), 7, 1, 12)
	if err != nil || mine.Total != 1 || mine.Items[0].Status != "uploading" {
		t.Fatalf("Mine = (%+v, %v)", mine, err)
	}
}

func TestPaginationValidation(t *testing.T) {
	svc := newTestService(&mockRepository{}, &mockObjectStore{})
	for _, args := range [][2]int{{0, 12}, {1, 0}, {1, 51}} {
		if _, err := svc.List(context.Background(), args[0], args[1]); !errors.Is(err, ErrPaginationInvalid) {
			t.Fatalf("List(%d,%d) error = %v", args[0], args[1], err)
		}
	}
}

func (m *mockRepository) ListPublic(ctx context.Context, query ListQuery) ([]Video, int64, error) {
	if m.listPublicFn != nil {
		return m.listPublicFn(ctx, query)
	}
	if m.listReadyFn != nil {
		return m.listReadyFn(ctx, query.Page, query.PageSize)
	}
	return []Video{}, 0, nil
}

func (m *mockRepository) FindPublicByID(ctx context.Context, id uint64) (*Video, error) {
	if m.findPublicByIDFn != nil {
		return m.findPublicByIDFn(ctx, id)
	}
	if m.findReadyByIDFn != nil {
		return m.findReadyByIDFn(ctx, id)
	}
	return nil, ErrNotFound
}

func (m *mockRepository) ViewerState(ctx context.Context, viewerID, videoID, authorID uint64) (*ViewerState, error) {
	if m.viewerStateFn == nil {
		return &ViewerState{}, nil
	}
	return m.viewerStateFn(ctx, viewerID, videoID, authorID)
}

func TestListQueryRejectsInvalidSearchAndSort(t *testing.T) {
	svc := newTestService(&mockRepository{}, &mockObjectStore{})
	for _, query := range []ListQuery{
		{Page: 1, PageSize: 12, Query: strings.Repeat("搜", 51)},
		{Page: 1, PageSize: 12, Sort: "trending"},
	} {
		if _, err := svc.ListPublic(context.Background(), query); !errors.Is(err, ErrListQueryInvalid) {
			t.Fatalf("ListPublic(%+v) error = %v, want ErrListQueryInvalid", query, err)
		}
	}
}

func TestEscapeLikeTreatsWildcardsAsLiteralCharacters(t *testing.T) {
	if got, want := escapeLike(`猫!_%`, '!'), `猫!!!_!%`; got != want {
		t.Fatalf("escapeLike = %q, want %q", got, want)
	}
}

func TestUpdateVideoRejectsInvalidOwnerOrPatch(t *testing.T) {
	ready := &Video{ID: 9, UserID: 7, Title: "原标题", Status: StatusReady, Visibility: VisibilityPrivate}
	deleted := &Video{ID: 9, UserID: 7, Status: StatusDeleted}
	cases := []struct {
		name    string
		userID  uint64
		videoID uint64
		video   *Video
		req     UpdateRequest
		want    error
	}{
		{name: "missing user", userID: 0, videoID: 9, video: ready, req: UpdateRequest{Title: strPtr("新")}, want: ErrUnauthorized},
		{name: "missing video", userID: 7, videoID: 0, video: ready, req: UpdateRequest{Title: strPtr("新")}, want: ErrNotFound},
		{name: "unknown video", userID: 7, videoID: 9, video: nil, req: UpdateRequest{Title: strPtr("新")}, want: ErrNotFound},
		{name: "non owner", userID: 8, videoID: 9, video: ready, req: UpdateRequest{Title: strPtr("新")}, want: ErrForbidden},
		{name: "already deleted", userID: 7, videoID: 9, video: deleted, req: UpdateRequest{Title: strPtr("新")}, want: ErrNotFound},
		{name: "empty patch", userID: 7, videoID: 9, video: ready, req: UpdateRequest{}, want: ErrPatchEmpty},
		{name: "blank title", userID: 7, videoID: 9, video: ready, req: UpdateRequest{Title: strPtr("   ")}, want: ErrTitleInvalid},
		{name: "long title", userID: 7, videoID: 9, video: ready, req: UpdateRequest{Title: strPtr(strings.Repeat("题", 101))}, want: ErrTitleInvalid},
		{name: "long description", userID: 7, videoID: 9, video: ready, req: UpdateRequest{Description: strPtr(strings.Repeat("介", 2001))}, want: ErrDescriptionInvalid},
		{name: "unknown visibility", userID: 7, videoID: 9, video: ready, req: UpdateRequest{Visibility: strPtr("unlisted")}, want: ErrVisibilityInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.findByIDFn = func(context.Context, uint64) (*Video, error) {
				if tc.video == nil {
					return nil, ErrNotFound
				}
				copy := *tc.video
				return &copy, nil
			}
			repo.updateOwnedFn = func(context.Context, uint64, uint64, VideoPatch) error {
				t.Fatal("invalid update must not reach the repository")
				return nil
			}
			_, err := newTestService(repo, &mockObjectStore{}).Update(context.Background(), tc.userID, tc.videoID, tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUpdateVideoAppliesTrimmedPatchWithoutTouchingAbsentFields(t *testing.T) {
	firstPublish := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	current := &Video{ID: 9, UserID: 7, Title: "旧标题", Description: "旧简介", Status: StatusReady, Visibility: VisibilityPrivate}
	var got VideoPatch
	repo := &mockRepository{}
	repo.findByIDFn = func(context.Context, uint64) (*Video, error) { return current, nil }
	repo.updateOwnedFn = func(_ context.Context, userID, videoID uint64, patch VideoPatch) error {
		if userID != 7 || videoID != 9 {
			t.Fatalf("UpdateOwned args = %d/%d", userID, videoID)
		}
		got = patch
		if patch.Title != nil {
			current.Title = *patch.Title
		}
		if patch.Description != nil {
			current.Description = *patch.Description
		}
		if patch.Visibility != nil {
			current.Visibility = *patch.Visibility
		}
		if current.PublishedAt == nil && current.Visibility == VisibilityPublic && current.Status == StatusReady {
			current.PublishedAt = &firstPublish
		}
		return nil
	}

	result, err := newTestService(repo, &mockObjectStore{}).Update(context.Background(), 7, 9, UpdateRequest{
		Title:      strPtr("  新标题  "),
		Visibility: strPtr("public"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title == nil || *got.Title != "新标题" {
		t.Fatalf("trimmed title patch = %v", got.Title)
	}
	if got.Description != nil {
		t.Fatalf("absent description must stay nil, got %q", *got.Description)
	}
	if got.Visibility == nil || *got.Visibility != VisibilityPublic {
		t.Fatalf("visibility patch = %v", got.Visibility)
	}
	if result.Visibility != "public" || result.PublishedAt == nil || !result.PublishedAt.Equal(firstPublish) {
		t.Fatalf("response = %+v", result)
	}
}

func TestUpdateVideoKeepsFirstPublishTimeWhenTogglingVisibility(t *testing.T) {
	firstPublish := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	current := &Video{ID: 9, UserID: 7, Status: StatusReady, Visibility: VisibilityPublic, PublishedAt: &firstPublish}
	repo := &mockRepository{}
	repo.findByIDFn = func(context.Context, uint64) (*Video, error) { return current, nil }
	repo.updateOwnedFn = func(_ context.Context, _, _ uint64, patch VideoPatch) error {
		current.Visibility = *patch.Visibility
		return nil
	}

	result, err := newTestService(repo, &mockObjectStore{}).Update(context.Background(), 7, 9, UpdateRequest{Visibility: strPtr("private")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if result.Visibility != "private" {
		t.Fatalf("visibility = %q", result.Visibility)
	}
	if result.PublishedAt == nil || !result.PublishedAt.Equal(firstPublish) {
		t.Fatalf("first publish time must be preserved, got %v", result.PublishedAt)
	}
}

func TestUpdateVideoRejectsMissingUserWithoutRepositoryWork(t *testing.T) {
	repo := &mockRepository{}
	repo.findByIDFn = func(context.Context, uint64) (*Video, error) {
		t.Fatal("missing user must not load a video")
		return nil, ErrNotFound
	}
	_, err := newTestService(repo, &mockObjectStore{}).Update(context.Background(), 0, 9, UpdateRequest{Title: strPtr("新")})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
}

func TestDeleteVideoRequiresOwnershipAndTreatsDeletedAsMissing(t *testing.T) {
	ready := &Video{ID: 9, UserID: 7, Status: StatusReady}
	cases := []struct {
		name    string
		userID  uint64
		videoID uint64
		video   *Video
		want    error
	}{
		{name: "missing user", userID: 0, videoID: 9, video: ready, want: ErrUnauthorized},
		{name: "missing video", userID: 7, videoID: 0, video: ready, want: ErrNotFound},
		{name: "unknown video", userID: 7, videoID: 9, video: nil, want: ErrNotFound},
		{name: "non owner", userID: 8, videoID: 9, video: ready, want: ErrForbidden},
		{name: "already deleted", userID: 7, videoID: 9, video: &Video{ID: 9, UserID: 7, Status: StatusDeleted}, want: ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.findByIDFn = func(context.Context, uint64) (*Video, error) {
				if tc.video == nil {
					return nil, ErrNotFound
				}
				copy := *tc.video
				return &copy, nil
			}
			repo.deleteOwnedFn = func(context.Context, uint64, uint64) error {
				t.Fatal("rejected delete must not reach the repository")
				return nil
			}
			err := newTestService(repo, &mockObjectStore{}).Delete(context.Background(), tc.userID, tc.videoID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDeleteVideoDelegatesToOwnerScopedDelete(t *testing.T) {
	repo := &mockRepository{}
	repo.findByIDFn = func(context.Context, uint64) (*Video, error) {
		return &Video{ID: 9, UserID: 7, Status: StatusReady}, nil
	}
	called := false
	repo.deleteOwnedFn = func(_ context.Context, userID, videoID uint64) error {
		called = true
		if userID != 7 || videoID != 9 {
			t.Fatalf("DeleteOwned args = %d/%d", userID, videoID)
		}
		return nil
	}
	if err := newTestService(repo, &mockObjectStore{}).Delete(context.Background(), 7, 9); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !called {
		t.Fatal("DeleteOwned was not called")
	}
}

func TestDetailForViewerIncludesViewerStateOnlyForLoggedInViewer(t *testing.T) {
	item := Video{ID: 42, UserID: 7, ObjectKey: "videos/7/42/source.mp4", Status: StatusReady, Visibility: VisibilityPublic}
	repo := &mockRepository{findPublicByIDFn: func(context.Context, uint64) (*Video, error) {
		copy := item
		return &copy, nil
	}}
	repo.viewerStateFn = func(_ context.Context, viewerID, videoID, authorID uint64) (*ViewerState, error) {
		if viewerID != 9 || videoID != 42 || authorID != 7 {
			t.Fatalf("ViewerState args = %d/%d/%d", viewerID, videoID, authorID)
		}
		return &ViewerState{Liked: true, Favorited: true, FollowingAuthor: true}, nil
	}
	svc := newTestService(repo, &mockObjectStore{})
	anonymous, err := svc.DetailForViewer(context.Background(), 0, 42)
	if err != nil || anonymous.ViewerState != nil {
		t.Fatalf("anonymous DetailForViewer = (%+v, %v)", anonymous, err)
	}
	viewer, err := svc.DetailForViewer(context.Background(), 9, 42)
	if err != nil || viewer.ViewerState == nil || !viewer.ViewerState.Liked {
		t.Fatalf("viewer DetailForViewer = (%+v, %v)", viewer, err)
	}
}
