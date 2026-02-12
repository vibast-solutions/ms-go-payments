FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /out/payments-service .

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=builder /out/payments-service /app/payments-service
USER appuser
EXPOSE 8080 9090
ENTRYPOINT ["/app/payments-service", "serve"]
