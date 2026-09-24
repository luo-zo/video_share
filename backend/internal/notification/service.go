package notification

import (
	"context"
	"time"
)

const (
	defaultPageSize = 20
	maxPageSize     = 50
)

type ListResponse struct {
	Items       []NotificationResponse `json:"items"`
	Page        int                    `json:"page"`
	PageSize    int                    `json:"page_size"`
	Total       int64                  `json:"total"`
	UnreadCount int64                  `json:"unread_count"`
}

type NotificationResponse struct {
	ID          uint64     `json:"id"`
	RecipientID uint64     `json:"recipient_id"`
	ActorID     *uint64    `json:"actor_id,omitempty"`
	Type        string     `json:"type"`
	VideoID     *uint64    `json:"video_id,omitempty"`
	CommentID   *uint64    `json:"comment_id,omitempty"`
	ReportID    *uint64    `json:"report_id,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, recipientID uint64, page, pageSize int) (*ListResponse, error) {
	if recipientID == 0 {
		return nil, ErrNotFound
	}
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return nil, ErrPaginationInvalid
	}
	items, total, unread, err := s.repo.List(ctx, recipientID, page, pageSize)
	if err != nil {
		return nil, err
	}
	result := make([]NotificationResponse, 0, len(items))
	for i := range items {
		item := items[i]
		result = append(result, NotificationResponse{ID: item.ID, RecipientID: item.RecipientID, ActorID: item.ActorID, Type: item.Type, VideoID: item.VideoID, CommentID: item.CommentID, ReportID: item.ReportID, ReadAt: item.ReadAt, CreatedAt: item.CreatedAt})
	}
	return &ListResponse{Items: result, Page: page, PageSize: pageSize, Total: total, UnreadCount: unread}, nil
}

func (s *Service) MarkRead(ctx context.Context, recipientID, id uint64) error {
	if recipientID == 0 || id == 0 {
		return ErrNotFound
	}
	return s.repo.MarkRead(ctx, recipientID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, recipientID uint64) (int64, error) {
	if recipientID == 0 {
		return 0, ErrNotFound
	}
	return s.repo.MarkAllRead(ctx, recipientID)
}
