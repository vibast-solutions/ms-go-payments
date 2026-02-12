package factory

import (
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

func NewModuleLogger(module string) logrus.FieldLogger {
	return logrus.WithField("module", module)
}

func LoggerWithContext(logger logrus.FieldLogger, ctx echo.Context) logrus.FieldLogger {
	return logger.WithField("request_id", ctx.Request().Header.Get("X-Request-ID"))
}
