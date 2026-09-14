package engagement

import (
	"time"

	"video_share/internal/video"
)

// RelationState 是一次点赞 / 收藏写入后的最终状态：当前用户是否处于该关系中，
// 以及该关系的最新总数。重复调用返回同样的结果。
type RelationState struct {
	Active bool   `json:"active"`
	Count  uint64 `json:"count"`
}

type CommentRequest struct {
	Content string `json:"content"`
}

type WatchRequest struct {
	ProgressMS uint64 `json:"progress_ms"`
	DurationMS uint64 `json:"duration_ms"`
}

type CommentResponse struct {
	ID        uint64                `json:"id"`
	VideoID   uint64                `json:"video_id"`
	UserID    uint64                `json:"user_id"`
	Content   string                `json:"content"`
	Author    *video.AuthorResponse `json:"author,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
}

type CommentListResponse struct {
	Items    []CommentResponse `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

func toCommentResponse(c *Comment) CommentResponse {
	result := CommentResponse{
		ID:        c.ID,
		VideoID:   c.VideoID,
		UserID:    c.UserID,
		Content:   c.Content,
		CreatedAt: c.CreatedAt,
	}
	if c.Author.ID != 0 {
		result.Author = &video.AuthorResponse{
			ID:       c.Author.ID,
			Username: c.Author.Username,
			Nickname: c.Author.Nickname,
		}
	}
	return result
}
