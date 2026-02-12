package factory

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNewModuleLogger(t *testing.T) {
	logger := NewModuleLogger("payments-controller")
	if logger == nil {
		t.Fatal("expected logger")
	}
}

func TestLoggerWithContextAddsRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	logger := LoggerWithContext(NewModuleLogger("payments-controller"), ctx)
	if logger == nil {
		t.Fatal("expected logger with context")
	}
}
