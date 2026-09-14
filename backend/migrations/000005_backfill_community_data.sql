-- +goose Up
UPDATE videos
SET published_at = COALESCE(processed_at, updated_at, created_at)
WHERE status = 2
  AND visibility = 1
  AND published_at IS NULL;

INSERT INTO video_stats (video_id)
SELECT id
FROM videos
ON DUPLICATE KEY UPDATE video_id = VALUES(video_id);

-- +goose Down
-- Irreversible data migration: published_at values and zero-valued statistics rows
-- cannot be distinguished safely from data created after this migration.
SELECT 1;
