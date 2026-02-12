# Payments Microservice - Claude Context

## Overview
Payments microservice with HTTP + gRPC APIs and worker commands.

It manages:
- payment creation and lookup
- provider callback ingestion and status transitions
- outbound status callback dispatch to caller services
- payment lifecycle jobs (reconcile/expire)

## Technology Stack
- Echo (HTTP)
- gRPC
- Cobra
- MySQL (`database/sql`)
- Logrus
- Internal API-key auth via `lib-go-auth`

## Module
- `github.com/vibast-solutions/ms-go-payments`

## Directory Structure
```
payments/
├── main.go
├── Makefile
├── cmd/
│   ├── root.go
│   ├── serve.go
│   ├── jobs.go
│   ├── logging.go
│   └── version.go
├── config/
│   └── config.go
├── proto/
│   └── payments.proto
├── app/
│   ├── controller/
│   │   └── payment.go
│   ├── grpc/
│   │   ├── interceptor.go
│   │   └── server.go
│   ├── service/
│   │   ├── errors.go
│   │   ├── payment.go
│   │   ├── callback.go
│   │   └── jobs.go
│   ├── mapper/
│   │   └── payment.go
│   ├── repository/
│   │   ├── common.go
│   │   ├── payment.go
│   │   ├── payment_event.go
│   │   └── payment_callback.go
│   ├── entity/
│   │   ├── payment.go
│   │   ├── payment_event.go
│   │   └── payment_callback.go
│   ├── provider/
│   │   ├── provider.go
│   │   ├── registry.go
│   │   └── stripe.go
│   ├── types/
│   │   ├── payments.go
│   │   ├── payments.pb.go
│   │   └── payments_grpc.pb.go
│   └── factory/
│       └── logger.go
└── scripts/
    └── gen_proto.sh
```

## Security
- HTTP middleware: `EchoInternalAuthMiddleware.RequireInternalAccess(APP_SERVICE_NAME)`
- gRPC middleware: `GRPCInternalAuthMiddleware.UnaryRequireInternalAccess(APP_SERVICE_NAME)`
- Request ID is mandatory on both transports.

## Commands
- `payments serve`
- `payments reconcile`
- `payments --worker reconcile`
- `payments callbacks dispatch`
- `payments --worker callbacks dispatch`
- `payments expire pending`
- `payments --worker expire pending`
- `payments version`
