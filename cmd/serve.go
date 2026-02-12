package cmd

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authclient "github.com/vibast-solutions/lib-go-auth/client"
	authmiddleware "github.com/vibast-solutions/lib-go-auth/middleware"
	authlibservice "github.com/vibast-solutions/lib-go-auth/service"
	"github.com/vibast-solutions/ms-go-payments/app/controller"
	paymentgrpc "github.com/vibast-solutions/ms-go-payments/app/grpc"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"github.com/vibast-solutions/ms-go-payments/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP and gRPC servers",
	Long:  "Start both HTTP (Echo) and gRPC servers for the payments service.",
	Run:   runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) {
	cfg, paymentService, cleanup := mustCreatePaymentService()
	defer cleanup()

	paymentController := controller.NewPaymentController(paymentService)
	grpcPaymentServer := paymentgrpc.NewServer(paymentService)

	authGRPCClient, err := authclient.NewGRPCClientFromAddr(context.Background(), cfg.InternalEndpoints.AuthGRPCAddr)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize auth gRPC client")
	}
	defer authGRPCClient.Close()

	internalAuthService := authlibservice.NewInternalAuthService(authGRPCClient)
	echoInternalAuthMiddleware := authmiddleware.NewEchoInternalAuthMiddleware(internalAuthService)
	grpcInternalAuthMiddleware := authmiddleware.NewGRPCInternalAuthMiddleware(internalAuthService)

	e := setupHTTPServer(paymentController, echoInternalAuthMiddleware, cfg.App.ServiceName)
	grpcSrv, lis := setupGRPCServer(cfg, grpcPaymentServer, grpcInternalAuthMiddleware, cfg.App.ServiceName)

	go func() {
		httpAddr := net.JoinHostPort(cfg.HTTP.Host, cfg.HTTP.Port)
		logrus.WithField("addr", httpAddr).Info("Starting HTTP server")
		if err := e.Start(httpAddr); err != nil && err != http.ErrServerClosed {
			logrus.WithError(err).Fatal("HTTP server error")
		}
	}()

	go func() {
		logrus.WithField("addr", lis.Addr().String()).Info("Starting gRPC server")
		if err := grpcSrv.Serve(lis); err != nil {
			logrus.WithError(err).Fatal("gRPC server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logrus.Info("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logrus.WithError(err).Warn("HTTP shutdown error")
	}
	grpcSrv.GracefulStop()

	logrus.Info("Server stopped")
}

func setupHTTPServer(
	paymentController *controller.PaymentController,
	internalAuthMiddleware *authmiddleware.EchoInternalAuthMiddleware,
	appServiceName string,
) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(echomiddleware.RequestLoggerWithConfig(echomiddleware.RequestLoggerConfig{
		LogURI:       true,
		LogStatus:    true,
		LogMethod:    true,
		LogRemoteIP:  true,
		LogLatency:   true,
		LogUserAgent: true,
		LogError:     true,
		HandleError:  true,
		LogRequestID: true,
		LogValuesFunc: func(_ echo.Context, v echomiddleware.RequestLoggerValues) error {
			fields := logrus.Fields{
				"remote_ip":  v.RemoteIP,
				"host":       v.Host,
				"method":     v.Method,
				"uri":        v.URI,
				"status":     v.Status,
				"latency":    v.Latency.String(),
				"latency_ns": v.Latency.Nanoseconds(),
				"user_agent": v.UserAgent,
			}
			entry := logrus.WithFields(fields)
			if v.Error != nil {
				entry = entry.WithError(v.Error)
			}
			entry.Info("http_request")
			return nil
		},
	}))
	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.CORS())
	e.Use(requireRequestID())
	e.Use(internalAuthMiddleware.RequireInternalAccess(appServiceName))

	e.GET("/health", paymentController.Health)

	payments := e.Group("/payments")
	payments.POST("", paymentController.CreatePayment)
	payments.GET("", paymentController.ListPayments)
	payments.GET("/:id", paymentController.GetPayment)
	payments.POST("/:id/cancel", paymentController.CancelPayment)

	webhooks := e.Group("/webhooks/providers")
	webhooks.POST("/:provider/:hash", paymentController.HandleProviderCallback)

	return e
}

func requireRequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			requestID := strings.TrimSpace(ctx.Request().Header.Get(echo.HeaderXRequestID))
			if requestID == "" {
				return ctx.JSON(http.StatusBadRequest, &types.ErrorResponse{Error: "x-request-id header is required"})
			}
			ctx.Response().Header().Set(echo.HeaderXRequestID, requestID)
			return next(ctx)
		}
	}
}

func setupGRPCServer(
	cfg *config.Config,
	paymentServer *paymentgrpc.Server,
	internalAuthMiddleware *authmiddleware.GRPCInternalAuthMiddleware,
	appServiceName string,
) (*grpc.Server, net.Listener) {
	grpcAddr := net.JoinHostPort(cfg.GRPC.Host, cfg.GRPC.Port)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to listen on gRPC port")
	}

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			paymentgrpc.RecoveryInterceptor(),
			paymentgrpc.RequestIDInterceptor(),
			paymentgrpc.LoggingInterceptor(),
			internalAuthMiddleware.UnaryRequireInternalAccess(appServiceName),
		),
	)
	types.RegisterPaymentsServiceServer(grpcSrv, paymentServer)

	return grpcSrv, lis
}

func mustCreatePaymentService() (*config.Config, *service.PaymentService, func()) {
	cfg, err := config.Load()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}
	if err := configureLogging(cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to configure logging")
	}

	db, err := sql.Open("mysql", cfg.MySQL.DSN)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to connect to database")
	}

	db.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.MySQL.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		logrus.WithError(err).Fatal("Failed to ping database")
	}

	paymentRepo := repository.NewPaymentRepository(db)
	eventRepo := repository.NewPaymentEventRepository(db)
	callbackRepo := repository.NewPaymentCallbackRepository(db)

	stripeProvider := provider.NewStripeProvider(provider.StripeConfig{
		SecretKey:                 cfg.Stripe.SecretKey,
		WebhookSecret:             cfg.Stripe.WebhookSecret,
		ProviderCallbackBaseURL:   cfg.Stripe.ProviderCallbackBaseURL,
		SignatureToleranceSeconds: cfg.Stripe.SignatureToleranceSeconds,
		HTTPTimeout:               cfg.Stripe.HTTPTimeout,
	})

	providerRegistry := provider.NewRegistry(stripeProvider)
	paymentService := service.NewPaymentService(
		paymentRepo,
		eventRepo,
		callbackRepo,
		providerRegistry,
		cfg.Payments,
		cfg.App.APIKey,
	)

	cleanup := func() {
		if err := db.Close(); err != nil {
			logrus.WithError(err).Warn("Failed to close database")
		}
	}

	return cfg, paymentService, cleanup
}
