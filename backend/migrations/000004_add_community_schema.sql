-- +goose Up
ALTER TABLE videos
    ADD COLUMN visibility TINYINT NOT NULL DEFAULT 1 COMMENT '1=public 2=private' AFTER status,
    ADD COLUMN published_at DATETIME(3) NULL AFTER processed_at,
    ADD CONSTRAINT chk_videos_visibility CHECK (visibility IN (1, 2)),
    ADD KEY idx_videos_discovery_latest (status, visibility, published_at, id);

CREATE TABLE video_stats (
    video_id       BIGINT UNSIGNED NOT NULL,
    view_count     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    like_count     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    favorite_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
    comment_count  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    updated_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (video_id),
    CONSTRAINT fk_video_stats_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE video_likes (
    user_id    BIGINT UNSIGNED NOT NULL,
    video_id   BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id, video_id),
    KEY idx_video_likes_video_created (video_id, created_at, user_id),
    CONSTRAINT fk_video_likes_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_video_likes_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE video_favorites (
    user_id    BIGINT UNSIGNED NOT NULL,
    video_id   BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id, video_id),
    KEY idx_video_favorites_user_created (user_id, created_at, video_id),
    KEY idx_video_favorites_video_created (video_id, created_at, user_id),
    CONSTRAINT fk_video_favorites_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_video_favorites_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE comments (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    video_id   BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    content    VARCHAR(500)    NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3)     NULL,
    PRIMARY KEY (id),
    KEY idx_comments_video_active_created (video_id, deleted_at, created_at, id),
    KEY idx_comments_user (user_id),
    CONSTRAINT fk_comments_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE,
    CONSTRAINT fk_comments_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_comments_content CHECK (CHAR_LENGTH(content) BETWEEN 1 AND 500)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE user_follows (
    follower_id BIGINT UNSIGNED NOT NULL,
    followee_id BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (follower_id, followee_id),
    KEY idx_user_follows_follower_created (follower_id, created_at, followee_id),
    KEY idx_user_follows_followee_created (followee_id, created_at, follower_id),
    CONSTRAINT fk_user_follows_follower FOREIGN KEY (follower_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_user_follows_followee FOREIGN KEY (followee_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_user_follows_not_self CHECK (follower_id <> followee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE watch_histories (
    user_id          BIGINT UNSIGNED NOT NULL,
    video_id         BIGINT UNSIGNED NOT NULL,
    progress_ms      BIGINT UNSIGNED NOT NULL DEFAULT 0,
    duration_ms      BIGINT UNSIGNED NOT NULL DEFAULT 0,
    first_watched_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_watched_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id, video_id),
    KEY idx_watch_histories_user_last (user_id, last_watched_at, video_id),
    KEY idx_watch_histories_video (video_id),
    CONSTRAINT fk_watch_histories_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_watch_histories_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE,
    CONSTRAINT chk_watch_histories_progress CHECK (duration_ms = 0 OR progress_ms <= duration_ms),
    CONSTRAINT chk_watch_histories_time CHECK (first_watched_at <= last_watched_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- 回滚会永久删除本阶段产生的互动关系、评论、观看历史和统计数据。
DROP TABLE IF EXISTS watch_histories;
DROP TABLE IF EXISTS user_follows;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS video_favorites;
DROP TABLE IF EXISTS video_likes;
DROP TABLE IF EXISTS video_stats;

ALTER TABLE videos
    DROP INDEX idx_videos_discovery_latest,
    DROP CHECK chk_videos_visibility,
    DROP COLUMN published_at,
    DROP COLUMN visibility;
