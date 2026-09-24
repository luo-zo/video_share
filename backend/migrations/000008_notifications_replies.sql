-- +goose Up

-- Threaded comments keep the original flat-comment API compatible: NULL parent_id
-- means a root comment, while root_id points at the visible thread root.
SET @replies_add_parent = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD COLUMN parent_id BIGINT UNSIGNED NULL AFTER user_id',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND column_name = 'parent_id'
);
PREPARE replies_add_parent_stmt FROM @replies_add_parent;
EXECUTE replies_add_parent_stmt;
DEALLOCATE PREPARE replies_add_parent_stmt;

SET @replies_add_root = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD COLUMN root_id BIGINT UNSIGNED NULL AFTER parent_id',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND column_name = 'root_id'
);
PREPARE replies_add_root_stmt FROM @replies_add_root;
EXECUTE replies_add_root_stmt;
DEALLOCATE PREPARE replies_add_root_stmt;

SET @replies_add_video_root_index = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD KEY idx_comments_video_root_created (video_id, root_id, created_at, id)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND index_name = 'idx_comments_video_root_created'
);
PREPARE replies_add_video_root_index_stmt FROM @replies_add_video_root_index;
EXECUTE replies_add_video_root_index_stmt;
DEALLOCATE PREPARE replies_add_video_root_index_stmt;

SET @replies_add_parent_index = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE comments ADD KEY idx_comments_parent_created (parent_id, created_at, id)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'comments' AND index_name = 'idx_comments_parent_created'
);
PREPARE replies_add_parent_index_stmt FROM @replies_add_parent_index;
EXECUTE replies_add_parent_index_stmt;
DEALLOCATE PREPARE replies_add_parent_index_stmt;

CREATE TABLE IF NOT EXISTS notifications (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  recipient_id BIGINT UNSIGNED NOT NULL,
  actor_id BIGINT UNSIGNED NULL,
  event_key VARCHAR(191) NOT NULL,
  type VARCHAR(32) NOT NULL,
  video_id BIGINT UNSIGNED NULL,
  comment_id BIGINT UNSIGNED NULL,
  report_id BIGINT UNSIGNED NULL,
  read_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_notifications_recipient_event (recipient_id, event_key),
  KEY idx_notifications_recipient_read (recipient_id, read_at, id),
  CONSTRAINT fk_notifications_recipient FOREIGN KEY (recipient_id) REFERENCES users(id),
  CONSTRAINT fk_notifications_actor FOREIGN KEY (actor_id) REFERENCES users(id),
  CONSTRAINT fk_notifications_video FOREIGN KEY (video_id) REFERENCES videos(id),
  CONSTRAINT fk_notifications_comment FOREIGN KEY (comment_id) REFERENCES comments(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS operation_receipts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  actor_id BIGINT UNSIGNED NOT NULL,
  action VARCHAR(64) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  request_hash CHAR(64) NOT NULL,
  resource_id BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_operation_receipts_actor_action_request (actor_id, action, request_id),
  KEY idx_operation_receipts_created (created_at),
  CONSTRAINT fk_operation_receipts_actor FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Existing comments are roots. This is deliberately a data-preserving backfill.
UPDATE comments SET root_id = id WHERE root_id IS NULL;

-- +goose Down
DROP TABLE IF EXISTS operation_receipts;
DROP TABLE IF EXISTS notifications;
ALTER TABLE comments
  DROP COLUMN root_id,
  DROP COLUMN parent_id;
