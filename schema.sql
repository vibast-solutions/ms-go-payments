CREATE DATABASE IF NOT EXISTS payments;
USE payments;

CREATE TABLE payments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    request_id VARCHAR(128) NOT NULL,
    caller_service VARCHAR(128) NOT NULL,
    resource_type VARCHAR(128) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    customer_ref VARCHAR(255) NULL,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,
    payment_method SMALLINT NOT NULL,
    payment_type SMALLINT NOT NULL,
    provider SMALLINT NOT NULL,
    recurring_interval VARCHAR(16) NULL,
    recurring_interval_count INT NULL,
    provider_payment_id VARCHAR(255) NULL,
    provider_subscription_id VARCHAR(255) NULL,
    checkout_url TEXT NULL,
    provider_callback_hash VARCHAR(128) NOT NULL,
    provider_callback_url VARCHAR(1024) NOT NULL,
    status_callback_url VARCHAR(1024) NOT NULL,
    refunded_cents BIGINT NOT NULL DEFAULT 0,
    refundable_cents BIGINT NOT NULL DEFAULT 0,
    metadata_json JSON NOT NULL,
    callback_delivery_status SMALLINT NOT NULL DEFAULT 0,
    callback_delivery_attempts INT NOT NULL DEFAULT 0,
    callback_delivery_next_at DATETIME NULL,
    callback_delivery_last_error VARCHAR(1024) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_payments_caller_request_id (caller_service, request_id),
    UNIQUE INDEX idx_payments_provider_callback_hash (provider, provider_callback_hash),
    INDEX idx_payments_status (status),
    INDEX idx_payments_provider (provider),
    INDEX idx_payments_resource (resource_type, resource_id),
    INDEX idx_payments_callback_delivery (callback_delivery_status, callback_delivery_next_at),
    INDEX idx_payments_updated_at (updated_at),
    INDEX idx_payments_created_at (created_at)
);

CREATE TABLE payment_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    payment_id BIGINT UNSIGNED NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    old_status SMALLINT NULL,
    new_status SMALLINT NOT NULL,
    provider_event_id VARCHAR(255) NULL,
    payload_json LONGTEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_payment_events_payment_id FOREIGN KEY (payment_id) REFERENCES payments(id) ON DELETE CASCADE,
    INDEX idx_payment_events_payment_id (payment_id),
    INDEX idx_payment_events_created_at (created_at)
);

CREATE TABLE payment_callbacks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    payment_id BIGINT UNSIGNED NULL,
    provider VARCHAR(32) NOT NULL,
    callback_hash VARCHAR(128) NOT NULL,
    signature VARCHAR(1024) NOT NULL,
    payload_json LONGTEXT NOT NULL,
    status SMALLINT NOT NULL,
    error VARCHAR(1024) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_payment_callbacks_payment_id FOREIGN KEY (payment_id) REFERENCES payments(id) ON DELETE SET NULL,
    INDEX idx_payment_callbacks_payment_id (payment_id),
    INDEX idx_payment_callbacks_provider_hash (provider, callback_hash),
    INDEX idx_payment_callbacks_status (status),
    INDEX idx_payment_callbacks_created_at (created_at)
);
