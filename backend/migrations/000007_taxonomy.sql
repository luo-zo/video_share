-- +goose Up
CREATE TABLE IF NOT EXISTS categories (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    slug          VARCHAR(64)     NOT NULL,
    name          VARCHAR(64)     NOT NULL,
    enabled       TINYINT(1)      NOT NULL DEFAULT 1,
    display_order INT            NOT NULL DEFAULT 0,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_categories_slug (slug),
    KEY idx_categories_enabled_order (enabled, display_order, id),
    CONSTRAINT chk_categories_enabled CHECK (enabled IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO categories (id, slug, name, enabled, display_order)
VALUES
    (1, 'uncategorized', '未分类', 1, 0),
    (2, 'life', '生活', 1, 10),
    (3, 'knowledge', '知识', 1, 20),
    (4, 'technology', '科技', 1, 30),
    (5, 'gaming', '游戏', 1, 40),
    (6, 'music', '音乐', 1, 50)
ON DUPLICATE KEY UPDATE
    name = VALUES(name), enabled = VALUES(enabled), display_order = VALUES(display_order);

CREATE TABLE IF NOT EXISTS tags (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    normalized_name VARCHAR(80)     NOT NULL,
    display_name    VARCHAR(80)     NOT NULL,
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_tags_normalized_name (normalized_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS video_tags (
    video_id   BIGINT UNSIGNED NOT NULL,
    tag_id     BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (video_id, tag_id),
    KEY idx_video_tags_tag_video (tag_id, video_id),
    CONSTRAINT fk_video_tags_video FOREIGN KEY (video_id) REFERENCES videos (id) ON DELETE CASCADE,
    CONSTRAINT fk_video_tags_tag FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @taxonomy_add_category = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE videos ADD COLUMN category_id BIGINT UNSIGNED NOT NULL DEFAULT 1 AFTER user_id',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND column_name = 'category_id'
);
PREPARE taxonomy_add_category_stmt FROM @taxonomy_add_category;
EXECUTE taxonomy_add_category_stmt;
DEALLOCATE PREPARE taxonomy_add_category_stmt;

SET @taxonomy_add_category_index = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE videos ADD KEY idx_videos_category_public (category_id, status, visibility, published_at, id)',
        'SELECT 1')
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND index_name = 'idx_videos_category_public'
);
PREPARE taxonomy_add_category_index_stmt FROM @taxonomy_add_category_index;
EXECUTE taxonomy_add_category_index_stmt;
DEALLOCATE PREPARE taxonomy_add_category_index_stmt;

SET @taxonomy_add_category_fk = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE videos ADD CONSTRAINT fk_videos_category FOREIGN KEY (category_id) REFERENCES categories (id)',
        'SELECT 1')
    FROM information_schema.table_constraints
    WHERE table_schema = DATABASE() AND table_name = 'videos' AND constraint_name = 'fk_videos_category'
);
PREPARE taxonomy_add_category_fk_stmt FROM @taxonomy_add_category_fk;
EXECUTE taxonomy_add_category_fk_stmt;
DEALLOCATE PREPARE taxonomy_add_category_fk_stmt;

-- +goose Down
ALTER TABLE videos DROP FOREIGN KEY fk_videos_category;
ALTER TABLE videos DROP INDEX idx_videos_category_public;
ALTER TABLE videos DROP COLUMN category_id;
DROP TABLE IF EXISTS video_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS categories;
