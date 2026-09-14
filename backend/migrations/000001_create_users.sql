-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username      VARCHAR(32)     NOT NULL,
    password_hash VARCHAR(255)    NOT NULL,
    nickname      VARCHAR(64)     NOT NULL,
    status        TINYINT         NOT NULL DEFAULT 1 COMMENT '1=正常(normal) 2=禁用(disabled)',
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- 回滚会删除 users 表及其中的全部数据（破坏性操作）
DROP TABLE IF EXISTS users;
