package ranking

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"video_share/internal/cache"
	"video_share/internal/user"
	"video_share/internal/video"
)

const (
	keyPrefix = "video_share:rank:"
	lockTTL   = 30 * time.Second
	// Rebuild runs every 60 seconds, so a three-minute TTL leaves two missed
	// cycles before a reader falls back to MySQL. Date-key retention is thus
	// bounded well below the eight-day operational maximum.
	snapshotTTL   = 3 * time.Minute
	metadataTTL   = 3 * time.Minute
	redisBatch    = int64(200)
	maxMetricRows = 100_000
	unlockLua     = `if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`
)

func snapshotKey(window Window, now time.Time) string {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	return keyPrefix + string(window) + ":" + now.In(zone).Format("2006-01-02")
}

func metadataKey(window Window, now time.Time) string {
	return snapshotKey(window, now) + ":generated_at"
}

type Repository interface {
	Rebuild(ctx context.Context, window Window, now time.Time) (time.Time, error)
	List(ctx context.Context, window Window, page, pageSize int, now time.Time) ([]Candidate, int64, time.Time, error)
}

type gormRepository struct {
	db             *gorm.DB
	redis          *cache.Client
	fallbackSlots  chan struct{}
	fallbackFlight singleflight.Group
}

func NewRepository(db *gorm.DB, redisClient *cache.Client) Repository {
	return &gormRepository{db: db, redis: redisClient, fallbackSlots: make(chan struct{}, 4)}
}

type metricRow struct {
	VideoID        uint64
	EffectiveViews uint64
	WatchTimeMS    uint64
	Completions    uint64
	NetLikes       int64
	NetFavorites   int64
	NetComments    int64
}

func (r *gormRepository) Rebuild(ctx context.Context, window Window, now time.Time) (time.Time, error) {
	if _, err := ParseWindow(string(window)); err != nil {
		return time.Time{}, err
	}
	if r.redis == nil {
		return time.Time{}, errors.New("ranking rebuild requires redis")
	}
	owner := uuid.NewString()
	lockKey := keyPrefix + "lock"
	acquired, err := r.redis.SetNX(ctx, lockKey, owner, lockTTL)
	if err != nil {
		return time.Time{}, fmt.Errorf("acquire ranking rebuild lock: %w", err)
	}
	if !acquired {
		return time.Time{}, ErrLockBusy
	}
	defer func() {
		_, _ = r.redis.Eval(context.Background(), unlockLua, []string{lockKey}, owner)
	}()

	rows, err := r.metricRows(ctx, window, now)
	if err != nil {
		return time.Time{}, err
	}
	tempKey := snapshotKey(window, now) + ":building:" + owner
	activeKey := snapshotKey(window, now)
	if err := r.redis.Delete(ctx, tempKey); err != nil && !cache.IsMiss(err) {
		return time.Time{}, fmt.Errorf("clear ranking temp key: %w", err)
	}
	if len(rows) > 0 {
		members := make([]cache.ZMember, 0, len(rows))
		for _, row := range rows {
			members = append(members, cache.ZMember{Score: Score(row.EffectiveViews, row.WatchTimeMS, row.Completions, row.NetLikes, row.NetFavorites, row.NetComments), Member: encodeMember(row.VideoID)})
		}
		if err := r.redis.ZAdd(ctx, tempKey, members...); err != nil {
			return time.Time{}, fmt.Errorf("write ranking temp key: %w", err)
		}
	}
	if err := r.redis.Expire(ctx, tempKey, snapshotTTL); err != nil && len(rows) > 0 {
		return time.Time{}, fmt.Errorf("expire ranking temp key: %w", err)
	}
	// RENAME is atomic. A failed build leaves the previous complete snapshot
	// untouched; an empty but valid build replaces it with an empty snapshot.
	if len(rows) == 0 {
		// Redis cannot rename a missing key. Create a short-lived empty marker.
		if err := r.redis.ZAdd(ctx, tempKey, cache.ZMember{Score: 0, Member: encodeMember(0)}); err != nil {
			return time.Time{}, fmt.Errorf("create empty ranking marker: %w", err)
		}
		if err := r.redis.Expire(ctx, tempKey, snapshotTTL); err != nil {
			return time.Time{}, fmt.Errorf("expire empty ranking marker: %w", err)
		}
	}
	published, err := r.redis.RenameIfOwner(ctx, lockKey, tempKey, activeKey, owner)
	if err != nil {
		return time.Time{}, fmt.Errorf("publish ranking snapshot: %w", err)
	}
	if !published {
		return time.Time{}, ErrLockLost
	}
	generatedAt := now.UTC()
	if err := r.redis.Set(ctx, metadataKey(window, now), generatedAt.Format(time.RFC3339Nano), metadataTTL); err != nil {
		return time.Time{}, fmt.Errorf("write ranking metadata: %w", err)
	}
	if err := r.pruneOldSnapshots(ctx, window, now); err != nil {
		return time.Time{}, fmt.Errorf("prune old ranking snapshots: %w", err)
	}
	return generatedAt, nil
}

func (r *gormRepository) pruneOldSnapshots(ctx context.Context, window Window, now time.Time) error {
	keys, err := r.redis.Scan(ctx, keyPrefix+string(window)+":*", 100)
	if err != nil {
		return err
	}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	cutoff := time.Date(now.In(zone).Year(), now.In(zone).Month(), now.In(zone).Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -7)
	prefix := keyPrefix + string(window) + ":"
	for _, key := range keys {
		datePart := strings.TrimPrefix(key, prefix)
		if len(datePart) < len("2006-01-02") {
			continue
		}
		date, parseErr := time.ParseInLocation("2006-01-02", datePart[:len("2006-01-02")], zone)
		if parseErr != nil || !date.Before(cutoff) {
			continue
		}
		if err := r.redis.Delete(ctx, key); err != nil && !cache.IsMiss(err) {
			return err
		}
	}
	return nil
}

func (r *gormRepository) List(ctx context.Context, window Window, page, pageSize int, now time.Time) ([]Candidate, int64, time.Time, error) {
	if _, err := ParseWindow(string(window)); err != nil {
		return nil, 0, time.Time{}, err
	}
	if page < 1 || pageSize < 1 || pageSize > 50 || page > 1_000_000 {
		return nil, 0, time.Time{}, errors.New("invalid ranking pagination")
	}
	if r.redis != nil {
		pageItems, total, generatedAt, err := r.redisCandidates(ctx, window, page, pageSize, now)
		if err == nil && !generatedAt.IsZero() {
			if total > 0 {
				return pageItems, total, generatedAt, nil
			}
			// A complete but empty/fully filtered snapshot is compliant but
			// unhelpful; show the latest public works with score zero.
			result, err := r.sharedFallback(ctx, "latest:"+fallbackKey(window, now, page, pageSize), func(loadCtx context.Context) (fallbackResult, error) {
				items, total, at, loadErr := r.latestPublic(loadCtx, page, pageSize, now)
				return fallbackResult{items: items, total: total, generatedAt: at}, loadErr
			})
			return result.items, result.total, result.generatedAt, err
		}
		// A missing snapshot or metadata is a cache miss, not an empty
		// authoritative ranking. Fall through to the MySQL rebuild view.
	}
	result, err := r.sharedFallback(ctx, "metrics:"+fallbackKey(window, now, page, pageSize), func(loadCtx context.Context) (fallbackResult, error) {
		var result []Candidate
		var total int64
		var generatedAt time.Time
		rows, metricErr := r.metricRows(loadCtx, window, now)
		if metricErr != nil {
			return fallbackResult{}, metricErr
		}
		candidates := make([]Candidate, 0, len(rows))
		for _, row := range rows {
			candidates = append(candidates, Candidate{VideoID: row.VideoID, Score: Score(row.EffectiveViews, row.WatchTimeMS, row.Completions, row.NetLikes, row.NetFavorites, row.NetComments)})
		}
		visible, filterErr := r.filterPublicCandidates(loadCtx, candidates)
		if filterErr != nil {
			return fallbackResult{}, filterErr
		}
		if len(visible) == 0 {
			var fallbackErr error
			result, total, generatedAt, fallbackErr = r.latestPublic(loadCtx, page, pageSize, now)
			return fallbackResult{items: result, total: total, generatedAt: generatedAt}, fallbackErr
		}
		total = int64(len(visible))
		result = pageCandidates(visible, page, pageSize)
		generatedAt = now.UTC()
		return fallbackResult{items: result, total: total, generatedAt: generatedAt}, nil
	})
	return result.items, result.total, result.generatedAt, err
}

// sharedFallback combines identical hot misses while keeping the loader alive
// if one waiting browser disconnects. The database work has its own deadline
// and still obeys the repository-wide concurrency limit.
func (r *gormRepository) sharedFallback(ctx context.Context, key string, load func(context.Context) (fallbackResult, error)) (fallbackResult, error) {
	resultCh := r.fallbackFlight.DoChan(key, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var result fallbackResult
		err := r.withFallbackSlot(loadCtx, func() error {
			var loadErr error
			result, loadErr = load(loadCtx)
			return loadErr
		})
		return result, err
	})
	select {
	case <-ctx.Done():
		return fallbackResult{}, ctx.Err()
	case outcome := <-resultCh:
		if outcome.Err != nil {
			return fallbackResult{}, outcome.Err
		}
		result, ok := outcome.Val.(fallbackResult)
		if !ok {
			return fallbackResult{}, errors.New("invalid ranking fallback result")
		}
		return result, nil
	}
}

type fallbackResult struct {
	items       []Candidate
	total       int64
	generatedAt time.Time
}

func fallbackKey(window Window, now time.Time, page, pageSize int) string {
	return fmt.Sprintf("%s:%s:%d:%d", window, snapshotKey(window, now), page, pageSize)
}

// withFallbackSlot bounds concurrent database rebuild views. A Redis outage
// must degrade to a bounded amount of MySQL work rather than allowing every
// request to aggregate the entire metrics table at once.
func (r *gormRepository) withFallbackSlot(ctx context.Context, fn func() error) error {
	if r.fallbackSlots == nil {
		// NewRepository always initializes this channel. Keep direct test
		// construction safe without changing the normal bounded path.
		return fn()
	}
	select {
	case r.fallbackSlots <- struct{}{}:
		defer func() { <-r.fallbackSlots }()
		return fn()
	default:
		return ErrFallbackBusy
	}
}

func (r *gormRepository) redisCandidates(ctx context.Context, window Window, page, pageSize int, now time.Time) ([]Candidate, int64, time.Time, error) {
	key := snapshotKey(window, now)
	generatedAt := time.Time{}
	if raw, metaErr := r.redis.Get(ctx, metadataKey(window, now)); metaErr == nil {
		generatedAt, _ = time.Parse(time.RFC3339Nano, raw)
	}
	if generatedAt.IsZero() {
		return nil, 0, generatedAt, nil
	}
	pageStart := (page - 1) * pageSize
	visibleTotal := 0
	pageItems := make([]Candidate, 0, pageSize)
	for start := int64(0); ; start += redisBatch {
		members, err := r.redis.ZRevRangeWithScores(ctx, key, start, start+redisBatch-1)
		if err != nil {
			return nil, 0, time.Time{}, err
		}
		if len(members) == 0 {
			break
		}
		batch := make([]Candidate, 0, len(members))
		for _, member := range members {
			id, decodeErr := decodeMember(member.Member)
			if decodeErr != nil || id == 0 {
				continue
			}
			batch = append(batch, Candidate{VideoID: id, Score: member.Score})
		}
		visible, err := r.filterPublicCandidates(ctx, batch)
		if err != nil {
			return nil, 0, time.Time{}, err
		}
		for _, candidate := range visible {
			if visibleTotal >= pageStart && len(pageItems) < pageSize {
				pageItems = append(pageItems, candidate)
			}
			visibleTotal++
		}
		if int64(len(members)) < redisBatch {
			break
		}
	}
	return pageItems, int64(visibleTotal), generatedAt, nil
}

func (r *gormRepository) metricRows(ctx context.Context, window Window, now time.Time) ([]metricRow, error) {
	start := window.start(now)
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(zone)
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	var rows []metricRow
	query := r.db.WithContext(ctx).Table("video_daily_metrics").Select(`
		video_id,
		COALESCE(SUM(effective_views), 0) AS effective_views,
		COALESCE(SUM(watch_time_ms), 0) AS watch_time_ms,
		COALESCE(SUM(completions), 0) AS completions,
		COALESCE(SUM(net_likes), 0) AS net_likes,
		COALESCE(SUM(net_favorites), 0) AS net_favorites,
		COALESCE(SUM(net_comments), 0) AS net_comments`).
		Where("stat_date BETWEEN ? AND ?", start, end).
		Group("video_id").Limit(maxMetricRows + 1)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load %s ranking metrics: %w", window, err)
	}
	if len(rows) > maxMetricRows {
		return nil, ErrTooManyRows
	}
	// SQL aggregation does not guarantee the tie order used by Redis, so use
	// the same deterministic order before the fallback path returns results.
	sort.Slice(rows, func(i, j int) bool {
		left := Score(rows[i].EffectiveViews, rows[i].WatchTimeMS, rows[i].Completions, rows[i].NetLikes, rows[i].NetFavorites, rows[i].NetComments)
		right := Score(rows[j].EffectiveViews, rows[j].WatchTimeMS, rows[j].Completions, rows[j].NetLikes, rows[j].NetFavorites, rows[j].NetComments)
		if left != right {
			return left > right
		}
		return rows[i].VideoID > rows[j].VideoID
	})
	return rows, nil
}

type publicVideoRow struct {
	ID                 uint64
	UserID             uint64
	CategoryID         uint64
	Title              string
	Description        string
	ObjectKey          string
	Status             video.Status
	Visibility         video.Visibility
	ModerationStatus   string
	FileSize           int64
	ContentType        string
	HLSMasterKey       *string
	CoverObjectKey     *string
	DurationMS         *uint64
	Width              *uint
	Height             *uint
	ProcessingProgress uint8
	ProcessingError    *string
	ProcessedAt        *time.Time
	PublishedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	AuthorID           uint64
	AuthorUsername     string
	AuthorNickname     string
	ViewCount          uint64
	LikeCount          uint64
	FavoriteCount      uint64
	CommentCount       uint64
}

func (r *gormRepository) filterPublicCandidates(ctx context.Context, candidates []Candidate) ([]Candidate, error) {
	if len(candidates) == 0 {
		return []Candidate{}, nil
	}
	ids := make([]uint64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.VideoID)
	}
	var rows []publicVideoRow
	if err := r.publicVideoQuery(ctx).Where("v.id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("filter public ranking videos: %w", err)
	}
	allowed := make(map[uint64]video.Video, len(rows))
	for _, row := range rows {
		allowed[row.ID] = row.toVideo()
	}
	filtered := make([]Candidate, 0, len(rows))
	for _, candidate := range candidates {
		if public, ok := allowed[candidate.VideoID]; ok {
			candidate.Video = public
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

const publicVideoSelect = `v.id, v.user_id, v.category_id, v.title, v.description, v.object_key,
	v.status, v.visibility, v.moderation_status, v.file_size, v.content_type, v.hls_master_key,
	v.cover_object_key, v.duration_ms, v.width, v.height, v.processing_progress, v.processing_error,
	v.processed_at, v.published_at, v.created_at, v.updated_at,
	u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname,
	COALESCE(s.view_count, 0) AS view_count, COALESCE(s.like_count, 0) AS like_count,
	COALESCE(s.favorite_count, 0) AS favorite_count, COALESCE(s.comment_count, 0) AS comment_count`

func (r *gormRepository) publicVideoQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("videos AS v").Select(publicVideoSelect).
		Joins("JOIN users AS u ON u.id = v.user_id").
		Joins("LEFT JOIN video_stats AS s ON s.video_id = v.id").
		Where("v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?", video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal)
}

// latestPublic is the deterministic score-zero fallback used when a ranking
// snapshot has no visible candidates. It intentionally reads MySQL so Redis
// never becomes the authority for publication or moderation state.
func (r *gormRepository) latestPublic(ctx context.Context, page, pageSize int, now time.Time) ([]Candidate, int64, time.Time, error) {
	var total int64
	countQuery := r.db.WithContext(ctx).Table("videos AS v").
		Joins("JOIN users AS u ON u.id = v.user_id").
		Where("v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?", video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("count latest public ranking videos: %w", err)
	}
	offset := (page - 1) * pageSize
	var rows []publicVideoRow
	if err := r.publicVideoQuery(ctx).
		Order("v.published_at DESC, v.id DESC").
		Offset(offset).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("load latest public ranking videos: %w", err)
	}
	items := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		items = append(items, Candidate{VideoID: row.ID, Score: 0, Video: row.toVideo()})
	}
	return items, total, now.UTC(), nil
}

func pageCandidates(candidates []Candidate, page, pageSize int) []Candidate {
	start := (page - 1) * pageSize
	if start >= len(candidates) {
		return []Candidate{}
	}
	end := start + pageSize
	if end > len(candidates) {
		end = len(candidates)
	}
	return candidates[start:end]
}

func (row publicVideoRow) toVideo() video.Video {
	return video.Video{
		ID: row.ID, UserID: row.UserID, CategoryID: row.CategoryID, Title: row.Title,
		Description: row.Description, ObjectKey: row.ObjectKey, Status: row.Status,
		Visibility: row.Visibility, ModerationStatus: row.ModerationStatus, FileSize: row.FileSize,
		ContentType: row.ContentType, HLSMasterKey: row.HLSMasterKey, CoverObjectKey: row.CoverObjectKey,
		DurationMS: row.DurationMS, Width: row.Width, Height: row.Height,
		ProcessingProgress: row.ProcessingProgress, ProcessingError: row.ProcessingError,
		ProcessedAt: row.ProcessedAt, PublishedAt: row.PublishedAt, CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Author:    video.Author{ID: row.AuthorID, Username: row.AuthorUsername, Nickname: row.AuthorNickname},
		Stats:     video.Stats{ViewCount: row.ViewCount, LikeCount: row.LikeCount, FavoriteCount: row.FavoriteCount, CommentCount: row.CommentCount},
	}
}
