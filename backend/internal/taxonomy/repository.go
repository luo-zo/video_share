package taxonomy

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCategoryNotFound = errors.New("category not found")
	ErrTagInvalid       = errors.New("invalid tag")
	ErrTooManyTags      = errors.New("too many tags")
)

type Repository interface {
	ListEnabledCategories(ctx context.Context) ([]Category, error)
	FindEnabledCategory(ctx context.Context, id uint64) (*Category, error)
	ApplyVideo(ctx context.Context, videoID, categoryID uint64, tags []NormalizedTag) error
	ForVideos(ctx context.Context, videoIDs []uint64) (map[uint64]VideoTaxonomy, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) ListEnabledCategories(ctx context.Context) ([]Category, error) {
	var rows []Category
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("display_order ASC, id ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list enabled categories: %w", err)
	}
	return rows, nil
}

func (r *gormRepository) FindEnabledCategory(ctx context.Context, id uint64) (*Category, error) {
	var row Category
	err := r.db.WithContext(ctx).Where("id = ? AND enabled = ?", id, true).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCategoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find category: %w", err)
	}
	return &row, nil
}

func (r *gormRepository) ApplyVideo(ctx context.Context, videoID, categoryID uint64, tags []NormalizedTag) error {
	if videoID == 0 || categoryID == 0 {
		return ErrCategoryNotFound
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var category Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND enabled = ?", categoryID, true).Take(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCategoryNotFound
			}
			return fmt.Errorf("lock category: %w", err)
		}
		if err := tx.Table("videos").Where("id = ?", videoID).Update("category_id", categoryID).Error; err != nil {
			return fmt.Errorf("update video category: %w", err)
		}
		if err := tx.Exec("DELETE FROM video_tags WHERE video_id = ?", videoID).Error; err != nil {
			return fmt.Errorf("replace video tags: %w", err)
		}
		for _, tag := range tags {
			if err := tx.Exec("INSERT INTO tags (normalized_name, display_name) VALUES (?, ?) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)", tag.Normalized, tag.Display).Error; err != nil {
				return fmt.Errorf("upsert video tag: %w", err)
			}
			var tagID uint64
			if err := tx.Table("tags").Where("normalized_name = ?", tag.Normalized).Pluck("id", &tagID).Error; err != nil {
				return fmt.Errorf("find video tag: %w", err)
			}
			if err := tx.Exec("INSERT IGNORE INTO video_tags (video_id, tag_id) VALUES (?, ?)", videoID, tagID).Error; err != nil {
				return fmt.Errorf("link video tag: %w", err)
			}
		}
		return nil
	})
}

func (r *gormRepository) ForVideos(ctx context.Context, videoIDs []uint64) (map[uint64]VideoTaxonomy, error) {
	result := make(map[uint64]VideoTaxonomy, len(videoIDs))
	if len(videoIDs) == 0 {
		return result, nil
	}
	for _, id := range videoIDs {
		result[id] = VideoTaxonomy{Tags: []Tag{}}
	}
	type categoryRow struct {
		VideoID uint64
		ID      uint64
		Slug    string
		Name    string
		Enabled bool
		Order   int
	}
	var categories []categoryRow
	if err := r.db.WithContext(ctx).Table("videos AS v").Select("v.id AS video_id, c.id, c.slug, c.name, c.enabled, c.display_order AS `order`").Joins("JOIN categories AS c ON c.id = v.category_id").Where("v.id IN ?", videoIDs).Scan(&categories).Error; err != nil {
		return nil, fmt.Errorf("load video categories: %w", err)
	}
	for _, row := range categories {
		copy := row
		result[row.VideoID] = VideoTaxonomy{Category: &Category{ID: copy.ID, Slug: copy.Slug, Name: copy.Name, Enabled: copy.Enabled, DisplayOrder: copy.Order}, Tags: result[row.VideoID].Tags}
	}
	type tagRow struct {
		VideoID uint64
		ID      uint64
		Norm    string
		Name    string
	}
	var tags []tagRow
	if err := r.db.WithContext(ctx).Table("video_tags AS vt").Select("vt.video_id, t.id, t.normalized_name AS norm, t.display_name AS name").Joins("JOIN tags AS t ON t.id = vt.tag_id").Where("vt.video_id IN ?", videoIDs).Order("vt.video_id ASC, t.id ASC").Scan(&tags).Error; err != nil {
		return nil, fmt.Errorf("load video tags: %w", err)
	}
	for _, row := range tags {
		value := result[row.VideoID]
		value.Tags = append(value.Tags, Tag{ID: row.ID, NormalizedName: row.Norm, DisplayName: row.Name})
		result[row.VideoID] = value
	}
	return result, nil
}
