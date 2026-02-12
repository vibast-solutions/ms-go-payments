# Payments E2E

Runs payments service end-to-end against MySQL using Docker Compose.

## Run

```bash
bash e2e/run.sh
```

The script:
1. Starts compose services from `e2e/docker-compose.yml`
2. Runs `go test ./e2e -tags e2e`
3. Tears down compose stack

## Ports

- HTTP: `48080`
- gRPC: `49090`
- MySQL: `43306`
