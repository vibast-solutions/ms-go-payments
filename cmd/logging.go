package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vibast-solutions/ms-go-payments/config"
)

func configureLogging(cfg *config.Config) error {
	level := strings.TrimSpace(cfg.Log.Level)
	if level == "" {
		level = "info"
	}
	parsed, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid LOG_LEVEL %q: %w", cfg.Log.Level, err)
	}
	logrus.SetLevel(parsed)
	logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})
	return nil
}
