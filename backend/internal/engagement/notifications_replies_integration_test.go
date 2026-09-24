//go:build integration

package engagement

import (
	"context"
	"errors"
	"testing"

	"video_share/internal/notification"
	"video_share/internal/video"
)

func TestCommentRepliesNotificationsAndReceipts(t *testing.T) {
	db := openEngagementTestDB(t)
	alice, bob := seedEngagementUsers(t, db)
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname) VALUES ('carol', 'hash', 'carol')`).Error; err != nil {
		t.Fatal(err)
	}
	carol := lookupEngagementID(t, db, "SELECT id FROM users WHERE username = 'carol'")
	videoID := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
	repo := NewRepository(db)
	ctx := context.Background()

	root, err := repo.(*gormRepository).CreateCommentWithRequest(ctx, bob, videoID, "根评论", nil, "comment-1")
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	replayed, err := repo.(*gormRepository).CreateCommentWithRequest(ctx, bob, videoID, "根评论", nil, "comment-1")
	if err != nil || replayed.ID != root.ID {
		t.Fatalf("replay = %+v, %v", replayed, err)
	}
	if _, err := repo.(*gormRepository).CreateCommentWithRequest(ctx, bob, videoID, "不同正文", nil, "comment-1"); !errors.Is(err, notification.ErrReceiptConflict) {
		t.Fatalf("conflict = %v", err)
	}

	parentID := root.ID
	reply, err := repo.(*gormRepository).CreateCommentWithRequest(ctx, carol, videoID, "回复", &parentID, "reply-1")
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if reply.RootID == nil || *reply.RootID != root.ID {
		t.Fatalf("reply root = %+v", reply)
	}

	var notifications int64
	if err := db.Raw("SELECT COUNT(*) FROM notifications").Scan(&notifications).Error; err != nil {
		t.Fatal(err)
	}
	if notifications != 3 {
		t.Fatalf("notifications = %d, want root author + reply author + video author", notifications)
	}
	items, total, err := repo.ListReplies(ctx, root.ID, 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Content != "回复" {
		t.Fatalf("replies = %#v, %d, %v", items, total, err)
	}
	if err := repo.(*gormRepository).DeleteComment(ctx, bob, root.ID); err != nil {
		t.Fatalf("delete root: %v", err)
	}
	items, total, err = repo.ListReplies(ctx, root.ID, 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Content != "回复" {
		t.Fatalf("replies after deleted root = %#v, %d, %v", items, total, err)
	}
}
