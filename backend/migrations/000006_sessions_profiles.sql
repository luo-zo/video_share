-- +goose Up
-- MySQL commits DDL implicitly. Keep each additive operation independently
-- retryable so a connection loss after DDL commit but before Goose records the
-- migration does not make the next `migrate up` fail on a duplicate column.
SET @sessions_add_bio = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN bio VARCHAR(200) NOT NULL DEFAULT '''' AFTER nickname',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = 'bio'
);
PREPARE sessions_add_bio_stmt FROM @sessions_add_bio;
EXECUTE sessions_add_bio_stmt;
DEALLOCATE PREPARE sessions_add_bio_stmt;

SET @sessions_add_role = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN role VARCHAR(32) NOT NULL DEFAULT ''user'' AFTER bio',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = 'role'
);
PREPARE sessions_add_role_stmt FROM @sessions_add_role;
EXECUTE sessions_add_role_stmt;
DEALLOCATE PREPARE sessions_add_role_stmt;

SET @sessions_add_role_check = (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE users ADD CONSTRAINT chk_users_role CHECK (role IN (''user'', ''admin''))',
        'SELECT 1')
    FROM information_schema.table_constraints
    WHERE table_schema = DATABASE() AND table_name = 'users' AND constraint_name = 'chk_users_role'
);
PREPARE sessions_add_role_check_stmt FROM @sessions_add_role_check;
EXECUTE sessions_add_role_check_stmt;
DEALLOCATE PREPARE sessions_add_role_check_stmt;

CREATE TABLE IF NOT EXISTS session_families (
    id                 CHAR(36)      NOT NULL,
    user_id            BIGINT UNSIGNED NOT NULL,
    created_at         DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    absolute_expires_at DATETIME(3)  NOT NULL,
    revoked_at         DATETIME(3)   NULL,
    PRIMARY KEY (id),
    KEY idx_session_families_user (user_id, revoked_at, absolute_expires_at),
    CONSTRAINT fk_session_families_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          CHAR(36)      NOT NULL,
    family_id   CHAR(36)      NOT NULL,
    token_hash  BINARY(32)    NOT NULL,
    created_at  DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    expires_at  DATETIME(3)   NOT NULL,
    rotated_at  DATETIME(3)   NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_refresh_tokens_hash (token_hash),
    KEY idx_refresh_tokens_family (family_id, rotated_at, expires_at),
    CONSTRAINT fk_refresh_tokens_family FOREIGN KEY (family_id) REFERENCES session_families (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS session_families;
SET @sessions_drop_role_check = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE users DROP CHECK chk_users_role',
        'SELECT 1')
    FROM information_schema.table_constraints
    WHERE table_schema = DATABASE() AND table_name = 'users' AND constraint_name = 'chk_users_role'
);
PREPARE sessions_drop_role_check_stmt FROM @sessions_drop_role_check;
EXECUTE sessions_drop_role_check_stmt;
DEALLOCATE PREPARE sessions_drop_role_check_stmt;

SET @sessions_drop_role = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE users DROP COLUMN role',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = 'role'
);
PREPARE sessions_drop_role_stmt FROM @sessions_drop_role;
EXECUTE sessions_drop_role_stmt;
DEALLOCATE PREPARE sessions_drop_role_stmt;

SET @sessions_drop_bio = (
    SELECT IF(COUNT(*) = 1,
        'ALTER TABLE users DROP COLUMN bio',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = 'bio'
);
PREPARE sessions_drop_bio_stmt FROM @sessions_drop_bio;
EXECUTE sessions_drop_bio_stmt;
DEALLOCATE PREPARE sessions_drop_bio_stmt;
