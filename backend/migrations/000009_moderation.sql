-- +goose Up
-- MySQL commits DDL implicitly. Keep each additive operation independently
-- retryable when a connection drops after DDL commits but before Goose records
-- the migration.
SET @moderation_add_video_status = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE videos ADD COLUMN moderation_status VARCHAR(16) NOT NULL DEFAULT ''visible'' AFTER visibility',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND column_name = 'moderation_status'
);
PREPARE moderation_add_video_status_stmt FROM @moderation_add_video_status;
EXECUTE moderation_add_video_status_stmt;
DEALLOCATE PREPARE moderation_add_video_status_stmt;

SET @moderation_add_video_index = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE videos ADD INDEX idx_videos_moderation_status (moderation_status)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND index_name = 'idx_videos_moderation_status'
);
PREPARE moderation_add_video_index_stmt FROM @moderation_add_video_index;
EXECUTE moderation_add_video_index_stmt;
DEALLOCATE PREPARE moderation_add_video_index_stmt;

SET @moderation_add_comment_status = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD COLUMN moderation_status VARCHAR(16) NOT NULL DEFAULT ''visible'' AFTER deleted_at',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND column_name = 'moderation_status'
);
PREPARE moderation_add_comment_status_stmt FROM @moderation_add_comment_status;
EXECUTE moderation_add_comment_status_stmt;
DEALLOCATE PREPARE moderation_add_comment_status_stmt;

SET @moderation_add_comment_index = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD INDEX idx_comments_moderation_status (moderation_status)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND index_name = 'idx_comments_moderation_status'
);
PREPARE moderation_add_comment_index_stmt FROM @moderation_add_comment_index;
EXECUTE moderation_add_comment_index_stmt;
DEALLOCATE PREPARE moderation_add_comment_index_stmt;

CREATE TABLE IF NOT EXISTS reports (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reporter_id BIGINT UNSIGNED NOT NULL,
  target_type VARCHAR(16) NOT NULL,
  target_id BIGINT UNSIGNED NOT NULL,
  reason_code VARCHAR(32) NOT NULL,
  detail VARCHAR(500) NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'open',
  assigned_to BIGINT UNSIGNED NULL,
  resolution_reason VARCHAR(500) NULL,
  action_id BIGINT UNSIGNED NULL,
  active_key VARCHAR(128) NULL,
  created_at DATETIME(3) NOT NULL,
  closed_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_reports_active (active_key),
  KEY idx_reports_reporter_created (reporter_id, created_at, id),
  KEY idx_reports_status_created (status, created_at, id),
  KEY idx_reports_target (target_type, target_id),
  CONSTRAINT fk_reports_reporter FOREIGN KEY (reporter_id) REFERENCES users(id),
  CONSTRAINT fk_reports_assigned FOREIGN KEY (assigned_to) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS moderation_actions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  actor_id BIGINT UNSIGNED NOT NULL,
  report_id BIGINT UNSIGNED NULL,
  target_type VARCHAR(16) NOT NULL,
  target_id BIGINT UNSIGNED NOT NULL,
  action VARCHAR(32) NOT NULL,
  reason VARCHAR(500) NOT NULL,
  before_state VARCHAR(32) NOT NULL,
  after_state VARCHAR(32) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_moderation_action_request (actor_id, action, request_id),
  KEY idx_moderation_actions_target (target_type, target_id, created_at, id),
  KEY idx_moderation_actions_actor (actor_id, created_at, id),
  KEY idx_moderation_actions_report (report_id),
  CONSTRAINT fk_moderation_actions_actor FOREIGN KEY (actor_id) REFERENCES users(id),
  CONSTRAINT fk_moderation_actions_report FOREIGN KEY (report_id) REFERENCES reports(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS moderation_actions;
DROP TABLE IF EXISTS reports;

SET @moderation_drop_comment_index = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE comments DROP INDEX idx_comments_moderation_status',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND index_name = 'idx_comments_moderation_status'
);
PREPARE moderation_drop_comment_index_stmt FROM @moderation_drop_comment_index;
EXECUTE moderation_drop_comment_index_stmt;
DEALLOCATE PREPARE moderation_drop_comment_index_stmt;

SET @moderation_drop_comment_status = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE comments DROP COLUMN moderation_status',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND column_name = 'moderation_status'
);
PREPARE moderation_drop_comment_status_stmt FROM @moderation_drop_comment_status;
EXECUTE moderation_drop_comment_status_stmt;
DEALLOCATE PREPARE moderation_drop_comment_status_stmt;

SET @moderation_drop_video_index = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE videos DROP INDEX idx_videos_moderation_status',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND index_name = 'idx_videos_moderation_status'
);
PREPARE moderation_drop_video_index_stmt FROM @moderation_drop_video_index;
EXECUTE moderation_drop_video_index_stmt;
DEALLOCATE PREPARE moderation_drop_video_index_stmt;

SET @moderation_drop_video_status = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE videos DROP COLUMN moderation_status',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND column_name = 'moderation_status'
);
PREPARE moderation_drop_video_status_stmt FROM @moderation_drop_video_status;
EXECUTE moderation_drop_video_status_stmt;
DEALLOCATE PREPARE moderation_drop_video_status_stmt;
