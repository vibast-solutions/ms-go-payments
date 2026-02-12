package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/config"
)

var (
	workerMode bool
)

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Reconcile stale provider-backed payments",
	Run: func(_ *cobra.Command, _ []string) {
		runCommand(
			"reconcile",
			func(cfg *config.Config) time.Duration { return cfg.Jobs.ReconcileInterval },
			func(s *service.PaymentService, ctx context.Context) error {
				return s.RunReconcileBatch(ctx)
			},
		)
	},
}

var callbacksCmd = &cobra.Command{
	Use:   "callbacks",
	Short: "Run status callback related commands",
}

var callbacksDispatchCmd = &cobra.Command{
	Use:   "dispatch",
	Short: "Dispatch pending terminal-status callbacks to caller services",
	Run: func(_ *cobra.Command, _ []string) {
		runCommand(
			"callbacks_dispatch",
			func(cfg *config.Config) time.Duration { return cfg.Jobs.CallbackDispatchInterval },
			func(s *service.PaymentService, ctx context.Context) error {
				return s.RunDispatchCallbacksBatch(ctx)
			},
		)
	},
}

var expireCmd = &cobra.Command{
	Use:   "expire",
	Short: "Run expiration-related commands",
}

var expirePendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "Mark long-running pending/processing payments as expired",
	Run: func(_ *cobra.Command, _ []string) {
		runCommand(
			"expire_pending",
			func(cfg *config.Config) time.Duration { return cfg.Jobs.ExpirePendingInterval },
			func(s *service.PaymentService, ctx context.Context) error {
				return s.RunExpirePendingBatch(ctx)
			},
		)
	},
}

func init() {
	rootCmd.AddCommand(reconcileCmd)
	rootCmd.AddCommand(callbacksCmd)
	rootCmd.AddCommand(expireCmd)
	callbacksCmd.AddCommand(callbacksDispatchCmd)
	expireCmd.AddCommand(expirePendingCmd)

	rootCmd.PersistentFlags().BoolVar(&workerMode, "worker", false, "Run continuously using configured interval")
}

func runCommand(
	name string,
	intervalResolver func(cfg *config.Config) time.Duration,
	fn func(s *service.PaymentService, ctx context.Context) error,
) {
	cfg, paymentService, cleanup := mustCreatePaymentService()
	defer cleanup()

	if workerMode {
		runWorker(name, intervalResolver(cfg), paymentService, fn)
		return
	}

	ctx := context.Background()
	runJob(name, func() error { return fn(paymentService, ctx) })
}

func runWorker(
	name string,
	interval time.Duration,
	paymentService *service.PaymentService,
	fn func(s *service.PaymentService, ctx context.Context) error,
) {
	if interval <= 0 {
		logrus.WithField("job", name).Fatal("invalid worker interval")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runJob(name, func() error { return fn(paymentService, ctx) })

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-quit:
			logrus.WithField("job", name).Info("Worker shutdown requested")
			return
		case <-ticker.C:
			runJob(name, func() error { return fn(paymentService, ctx) })
		}
	}
}

func runJob(name string, fn func() error) {
	start := time.Now()
	err := fn()
	latency := time.Since(start)
	if err != nil {
		logrus.WithError(err).WithField("job", name).WithField("latency", latency.String()).Error("job_failed")
		return
	}
	logrus.WithField("job", name).WithField("latency", latency.String()).Info("job_completed")
}
