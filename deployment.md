# Payments Deployment Notes

## Prerequisites

- MySQL 8+
- Reachable Auth gRPC service
- Stripe credentials (`STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`)
- Public callback base URL routed to your internal callback proxy (`PAYMENTS_PROVIDER_CALLBACK_BASE_URL`)

## Database Setup

Apply `schema.sql`:

```bash
mysql -u <user> -p < payments/schema.sql
```

## Environment

Start from `.env.example` and provide real values for:

- `MYSQL_DSN`
- `APP_API_KEY`
- `AUTH_SERVICE_GRPC_ADDR`
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `PAYMENTS_PROVIDER_CALLBACK_BASE_URL`

## Start API

```bash
./build/payments-service serve
```

## Start Workers

Run each worker independently so schedules can differ:

```bash
./build/payments-service --worker reconcile
./build/payments-service --worker callbacks dispatch
./build/payments-service --worker expire pending
```

## Health

- HTTP: `GET /health`
- gRPC: `Health`

All endpoints require internal auth (`x-api-key`) and request-id (`X-Request-ID` / `x-request-id`).
