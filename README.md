# Payments Microservice

`github.com/vibast-solutions/ms-go-payments`

Payments microservice for internal service-to-service payment orchestration over HTTP and gRPC.

It follows the same architecture and security model as the other services in this repository:
- HTTP and gRPC in one process
- Internal API-key verification via Auth service (`lib-go-auth` middleware)
- Layered architecture (`controller/grpc -> service -> repository -> entity`)

## What It Supports

- Create hosted Stripe payments (`hosted_card` and `payment_link`)
- One-time and recurring payment intents
- Idempotency via mandatory `request_id` + `caller_service`
- Payment retrieval and listing
- Cancel non-paid payments
- Provider callback handling (`/webhooks/providers/:provider/:hash`)
- Worker jobs for:
  - stale payment reconcile against provider
  - dispatching terminal payment status callbacks to caller services
  - expiring stuck pending/processing payments

## Security Model

- Every HTTP route and every gRPC method is protected by internal API-key middleware.
- `X-Request-ID` is mandatory for all HTTP requests.
- `x-request-id` metadata is mandatory for all gRPC requests.
- `request_id` is required in `CreatePaymentRequest` and is used for idempotency.

## Build

```bash
make build
make build-all
```

## Run

```bash
# Start HTTP + gRPC API
./build/payments-service serve

# One-off jobs
./build/payments-service reconcile
./build/payments-service callbacks dispatch
./build/payments-service expire pending

# Worker mode (per command)
./build/payments-service reconcile --worker
./build/payments-service callbacks dispatch --worker
./build/payments-service expire pending --worker
```

Or with `go run`:

```bash
go run main.go serve
go run main.go reconcile
go run main.go callbacks dispatch
go run main.go expire pending
go run main.go reconcile --worker
go run main.go callbacks dispatch --worker
go run main.go expire pending --worker
```

## CLI Commands

- `serve`
  - Starts HTTP and gRPC servers.
- `reconcile`
  - Reconciles stale `pending/processing` provider-backed payments against provider status.
  - `--worker` repeats using `PAYMENTS_RECONCILE_INTERVAL_MINUTES`.
- `callbacks dispatch`
  - Dispatches terminal payment status payloads to caller-defined `status_callback_url`.
  - Retries and marks delivery failed when retry budget is exhausted.
  - `--worker` repeats using `PAYMENTS_CALLBACK_DISPATCH_INTERVAL_MINUTES`.
- `expire pending`
  - Marks long-running `pending/processing` payments as `expired`.
  - `--worker` repeats using `PAYMENTS_EXPIRE_PENDING_INTERVAL_MINUTES`.
- `version`
  - Prints version/build metadata.

## Configuration

Use `.env.example` as baseline. Key variables:

- Core: `APP_SERVICE_NAME`, `APP_API_KEY`, `AUTH_SERVICE_GRPC_ADDR`
- Network: `HTTP_HOST`, `HTTP_PORT`, `GRPC_HOST`, `GRPC_PORT`
- DB: `MYSQL_DSN`, pool configuration vars
- Stripe: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `PAYMENTS_PROVIDER_CALLBACK_BASE_URL`
- Job/runtime tuning: `PAYMENTS_*`

## HTTP API

HTTP binds request payloads into protobuf-generated request structs (`app/types`) and returns protobuf-generated response structs as JSON. HTTP and gRPC call the same underlying service methods.

Routes:

- `GET /health`
- `POST /payments`
- `GET /payments/:id`
- `GET /payments`
- `POST /payments/:id/cancel`
- `POST /webhooks/providers/:provider/:hash`

Headers:

- `X-API-Key: <caller-api-key>`
- `X-Request-ID: <unique-request-id>`

## gRPC API

Service: `payments.PaymentsService`

Methods:
- `Health`
- `CreatePayment`
- `GetPayment`
- `ListPayments`
- `CancelPayment`
- `HandleProviderCallback`

Generate protobuf files:

```bash
PATH="$HOME/go/bin:$PATH" ./scripts/gen_proto.sh
```

## Database

See:
- `schema.sql`
- `deployment.md`

## E2E

```bash
bash e2e/run.sh
```

This spins up MySQL + payments service via Docker Compose and runs `go test ./e2e -tags e2e`.
