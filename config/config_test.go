package config

import (
	"os"
	"testing"
	"time"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s failed: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		}
	})
}

func TestLoadRequiresMySQLDSN(t *testing.T) {
	unsetEnv(t, "MYSQL_DSN")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing MYSQL_DSN")
	}
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	setEnv(t, "MYSQL_DSN", "root:root@tcp(localhost:3306)/payments?parseTime=true")
	setEnv(t, "APP_SERVICE_NAME", "payments-test")
	setEnv(t, "HTTP_PORT", "8181")
	setEnv(t, "GRPC_PORT", "9191")
	setEnv(t, "MYSQL_MAX_OPEN_CONNS", "20")
	setEnv(t, "MYSQL_MAX_IDLE_CONNS", "8")
	setEnv(t, "MYSQL_CONN_MAX_LIFETIME_MINUTES", "40")
	setEnv(t, "PAYMENTS_CALLBACK_MAX_ATTEMPTS", "5")
	setEnv(t, "PAYMENTS_CALLBACK_RETRY_INTERVAL_MINUTES", "7")
	setEnv(t, "PAYMENTS_PENDING_TIMEOUT_MINUTES", "11")
	setEnv(t, "PAYMENTS_RECONCILE_STALE_AFTER_MINUTES", "13")
	setEnv(t, "PAYMENTS_JOB_BATCH_SIZE", "99")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.App.ServiceName != "payments-test" {
		t.Fatalf("unexpected app service name: %s", cfg.App.ServiceName)
	}
	if cfg.HTTP.Port != "8181" || cfg.GRPC.Port != "9191" {
		t.Fatalf("unexpected ports: http=%s grpc=%s", cfg.HTTP.Port, cfg.GRPC.Port)
	}
	if cfg.MySQL.MaxOpenConns != 20 || cfg.MySQL.MaxIdleConns != 8 {
		t.Fatalf("unexpected mysql pool config: %+v", cfg.MySQL)
	}
	if cfg.MySQL.ConnMaxLifetime != 40*time.Minute {
		t.Fatalf("unexpected mysql lifetime: %v", cfg.MySQL.ConnMaxLifetime)
	}
	if cfg.Payments.CallbackMaxAttempts != 5 {
		t.Fatalf("unexpected callback max attempts: %d", cfg.Payments.CallbackMaxAttempts)
	}
	if cfg.Payments.CallbackRetryInterval != 7*time.Minute {
		t.Fatalf("unexpected callback retry interval: %v", cfg.Payments.CallbackRetryInterval)
	}
	if cfg.Payments.PendingTimeout != 11*time.Minute {
		t.Fatalf("unexpected pending timeout: %v", cfg.Payments.PendingTimeout)
	}
	if cfg.Payments.ReconcileStaleAfter != 13*time.Minute {
		t.Fatalf("unexpected reconcile stale after: %v", cfg.Payments.ReconcileStaleAfter)
	}
	if cfg.Payments.JobBatchSize != 99 {
		t.Fatalf("unexpected job batch size: %d", cfg.Payments.JobBatchSize)
	}
}
