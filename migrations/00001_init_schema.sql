-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    username      VARCHAR(64)  NOT NULL UNIQUE,
    email         VARCHAR(255) NOT NULL UNIQUE,
    phone_e164    VARCHAR(20),
    password_hash VARCHAR(255) NOT NULL,
    role          ENUM('admin','operator','viewer') NOT NULL DEFAULT 'operator',
    status        ENUM('active','inactive')         NOT NULL DEFAULT 'active',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_users_status (status)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_project_alert_subscription (
    id               BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id          BIGINT       NOT NULL,
    project_id       VARCHAR(64)  NOT NULL,
    min_alert_level  ENUM('blue','orange','red') NOT NULL DEFAULT 'blue',
    sms_enabled      TINYINT(1)   NOT NULL DEFAULT 0,
    push_enabled     TINYINT(1)   NOT NULL DEFAULT 0,
    email_enabled    TINYINT(1)   NOT NULL DEFAULT 0,
    quiet_hours_start TIME,
    quiet_hours_end   TIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_sub_user_project (user_id, project_id),
    INDEX idx_sub_project (project_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_device (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT       NOT NULL,
    device_token VARCHAR(255) NOT NULL UNIQUE,
    platform     ENUM('android','ios','web') NOT NULL,
    app_version  VARCHAR(32),
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status       ENUM('active','revoked') NOT NULL DEFAULT 'active',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_device_user (user_id),
    INDEX idx_device_user_status (user_id, status)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_device;
DROP TABLE IF EXISTS user_project_alert_subscription;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
