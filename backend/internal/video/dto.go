package video

import (
	"fmt"
	"time"
)

type CreateRequest struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description"`
	FileName    string   `json:"file_name" binding:"required"`
	ContentType string   `json:"content_type" binding:"required"`
	FileSize    int64    `json:"file_size" binding:"required"`
	CategoryID  uint64   `json:"category_id"`
	Tags        []string `json:"tags"`
}

// UpdateRequest is an author's partial update. Pointer fields distinguish a
// field that was left out from one that was supplied with an empty value.
type UpdateRequest struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Visibility  *string   `json:"visibility"`
	CategoryID  *uint64   `json:"category_id"`
	Tags        *[]string `json:"tags"`
}

// VideoPatch carries only the validated fields an author chose to change.
type VideoPatch struct {
	Title       *string
	Description *string
	Visibility  *Visibility
	CategoryID  *uint64
	Tags        *[]string
}

// Sort selects the public discovery ordering.
type Sort string

const (
	SortLatest  Sort = "latest"
	SortPopular Sort = "popular"
)

// ListQuery is the validated public discovery request. Query is already
// trimmed and length-checked by the Service before it reaches the Repository.
type ListQuery struct {
	Page       int
	PageSize   int
	Query      string
	Sort       Sort
	CategoryID uint64
	Tags       []string
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
	CategoryID  uint64          `json:"category_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Visibility  string          `json:"visibility"`
	FileSize    int64           `json:"file_size"`
	ContentType string          `json:"content_type"`
	CoverURL    *string         `json:"cover_url"`
	DurationMS  *uint64         `json:"duration_ms"`
	Width       *uint           `json:"width"`
	Height      *uint           `json:"height"`
	PublishedAt *time.Time      `json:"published_at"`
	Stats       Stats           `json:"stats"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Author      *AuthorResponse `json:"author,omitempty"`
	Category    *CategoryInfo   `json:"category,omitempty"`
	Tags        []TagInfo       `json:"tags,omitempty"`
}

type CreateResponse struct {
	VideoResponse
	UploadURL       string `json:"upload_url"`
	UploadExpiresIn int64  `json:"upload_expires_in"`
}

type DetailResponse struct {
	VideoResponse
	PlayURL       string       `json:"play_url"`
	PlayType      string       `json:"play_type"`
	PlayExpiresIn int64        `json:"play_expires_in"`
	ViewerState   *ViewerState `json:"viewer_state,omitempty"`
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
		CategoryID:  v.CategoryID,
		Title:       v.Title,
		Description: v.Description,
		Status:      v.Status.String(),
		Visibility:  v.Visibility.String(),
		FileSize:    v.FileSize,
		ContentType: v.ContentType,
		DurationMS:  v.DurationMS,
		Width:       v.Width,
		Height:      v.Height,
		PublishedAt: v.PublishedAt,
		Stats:       v.Stats,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
		Category:    v.Category,
		Tags:        v.Tags,
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

// ToVideoResponses projects loaded videos into their public response shape. It
// is exported so other modules can reuse the projection instead of duplicating
// the cover URL and author mapping rules.
func ToVideoResponses(videos []Video) []VideoResponse {
	return toVideoResponses(videos)
}
