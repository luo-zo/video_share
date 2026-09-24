-- +goose Up
-- MySQL commits DDL implicitly. Each additive column is independently
-- retryable so a connection loss cannot leave Goose permanently blocked.
SET @metrics_add_history_credit = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE watch_histories ADD COLUMN effective_watch_ms BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER duration_ms',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'watch_histories' AND column_name = 'effective_watch_ms'
);
PREPARE metrics_add_history_credit_stmt FROM @metrics_add_history_credit;
EXECUTE metrics_add_history_credit_stmt;
DEALLOCATE PREPARE metrics_add_history_credit_stmt;

SET @metrics_add_history_credit_at = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE watch_histories ADD COLUMN last_watch_credit_at DATETIME(3) NULL AFTER effective_watch_ms',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'watch_histories' AND column_name = 'last_watch_credit_at'
);
PREPARE metrics_add_history_credit_at_stmt FROM @metrics_add_history_credit_at;
EXECUTE metrics_add_history_credit_at_stmt;
DEALLOCATE PREPARE metrics_add_history_credit_at_stmt;

CREATE TABLE IF NOT EXISTS watch_sessions (
  id                 CHAR(36) NOT NULL,
  user_id            BIGINT UNSIGNED NOT NULL,
  video_id           BIGINT UNSIGNED NOT NULL,
  created_at         DATETIME(3) NOT NULL,
  expires_at         DATETIME(3) NOT NULL,
  last_seq           BIGINT UNSIGNED NOT NULL DEFAULT 0,
  last_position_ms   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  duration_ms        BIGINT UNSIGNED NOT NULL DEFAULT 0,
  last_received_at   DATETIME(3) NOT NULL,
  credited_ms        BIGINT UNSIGNED NOT NULL DEFAULT 0,
  qualified_at       DATETIME(3) NULL,
  completed_at       DATETIME(3) NULL,
  cohort_date        DATE NULL,
  PRIMARY KEY (id),
  KEY idx_watch_sessions_user_created (user_id, created_at, id),
  KEY idx_watch_sessions_video_created (video_id, created_at, id),
  CONSTRAINT fk_watch_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_watch_sessions_video FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS video_daily_metrics (
  video_id          BIGINT UNSIGNED NOT NULL,
  stat_date         DATE NOT NULL,
  effective_views   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  watch_time_ms     BIGINT UNSIGNED NOT NULL DEFAULT 0,
  completions       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  net_likes         BIGINT NOT NULL DEFAULT 0,
  net_favorites     BIGINT NOT NULL DEFAULT 0,
  net_comments      BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (video_id, stat_date),
  KEY idx_video_daily_metrics_date (stat_date, video_id),
  CONSTRAINT fk_video_daily_metrics_video FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS creator_daily_metrics (
  creator_id        BIGINT UNSIGNED NOT NULL,
  stat_date         DATE NOT NULL,
  effective_views   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  watch_time_ms     BIGINT UNSIGNED NOT NULL DEFAULT 0,
  completions       BIGINT UNSIGNED NOT NULL DEFAULT 0,
  net_likes         BIGINT NOT NULL DEFAULT 0,
  net_favorites     BIGINT NOT NULL DEFAULT 0,
  net_comments      BIGINT NOT NULL DEFAULT 0,
  net_followers     BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (creator_id, stat_date),
  KEY idx_creator_daily_metrics_date (stat_date, creator_id),
  CONSTRAINT fk_creator_daily_metrics_creator FOREIGN KEY (creator_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS creator_daily_metrics;
DROP TABLE IF EXISTS video_daily_metrics;
DROP TABLE IF EXISTS watch_sessions;

SET @metrics_drop_history_credit_at = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE watch_histories DROP COLUMN last_watch_credit_at',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'watch_histories' AND column_name = 'last_watch_credit_at'
);
PREPARE metrics_drop_history_credit_at_stmt FROM @metrics_drop_history_credit_at;
EXECUTE metrics_drop_history_credit_at_stmt;
DEALLOCATE PREPARE metrics_drop_history_credit_at_stmt;

SET @metrics_drop_history_credit = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE watch_histories DROP COLUMN effective_watch_ms',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'watch_histories' AND column_name = 'effective_watch_ms'
);
PREPARE metrics_drop_history_credit_stmt FROM @metrics_drop_history_credit;
EXECUTE metrics_drop_history_credit_stmt;
DEALLOCATE PREPARE metrics_drop_history_credit_stmt;
