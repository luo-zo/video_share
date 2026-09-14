package video

import (
	"fmt"
	"time"
)

type CreateRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	FileSize    int64  `json:"file_size" binding:"required"`
}

type AuthorResponse struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// VideoResponse deliberately excludes ObjectKey. Clients receive only
// short-lived presigned URLs for object access.
type VideoResponse struct {
	ID          uint64          `json:"id"`
	UserID      uint64          `json:"user_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	FileSize    int64           `json:"file_size"`
	ContentType string          `json:"content_type"`
	CoverURL    *string         `json:"cover_url"`
	DurationMS  *uint64         `json:"duration_ms"`
	Width       *uint           `json:"width"`
	Height      *uint           `json:"height"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Author      *AuthorResponse `json:"author,omitempty"`
}

type CreateResponse struct {
	VideoResponse
	UploadURL       string `json:"upload_url"`
	UploadExpiresIn int64  `json:"upload_expires_in"`
}

type DetailResponse struct {
	VideoResponse
	PlayURL       string `json:"play_url"`
	PlayType      string `json:"play_type"`
	PlayExpiresIn int64  `json:"play_expires_in"`
}

type OwnerVideoResponse struct {
	VideoResponse
	ProcessingProgress uint8   `json:"processing_progress"`
	ProcessingError    *string `json:"processing_error"`
}

type OwnerListResponse struct {
	Items    []OwnerVideoResponse `json:"items"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Total    int64                `json:"total"`
}

type ListResponse struct {
	Items    []VideoResponse `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

func toVideoResponse(v *Video) VideoResponse {
	result := VideoResponse{
		ID:          v.ID,
		UserID:      v.UserID,
		Title:       v.Title,
		Description: v.Description,
		Status:      v.Status.String(),
		FileSize:    v.FileSize,
		ContentType: v.ContentType,
		DurationMS:  v.DurationMS,
		Width:       v.Width,
		Height:      v.Height,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
	if v.CoverObjectKey != nil && *v.CoverObjectKey != "" {
		coverURL := fmt.Sprintf("/api/v1/videos/%d/cover", v.ID)
		result.CoverURL = &coverURL
	}
	if v.Author.ID != 0 {
		result.Author = &AuthorResponse{
			ID:       v.Author.ID,
			Username: v.Author.Username,
			Nickname: v.Author.Nickname,
		}
	}
	return result
}

func toOwnerVideoResponse(v *Video) OwnerVideoResponse {
	return OwnerVideoResponse{
		VideoResponse:      toVideoResponse(v),
		ProcessingProgress: v.ProcessingProgress,
		ProcessingError:    v.ProcessingError,
	}
}

func toOwnerVideoResponses(videos []Video) []OwnerVideoResponse {
	result := make([]OwnerVideoResponse, 0, len(videos))
	for i := range videos {
		result = append(result, toOwnerVideoResponse(&videos[i]))
	}
	return result
}

func toVideoResponses(videos []Video) []VideoResponse {
	result := make([]VideoResponse, 0, len(videos))
	for i := range videos {
		result = append(result, toVideoResponse(&videos[i]))
	}
	return result
}
