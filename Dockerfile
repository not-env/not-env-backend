# Build stage
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments
# VERSION: Version to inject into binary (default: 0.1.0)
ARG VERSION=0.1.0

# Build the application with version (CGO disabled - PostgreSQL/MySQL drivers are pure Go)
# Required environment variables at runtime:
#   DB_TYPE: Database type (postgres or mysql)
#   DB_HOST: Database host
#   DB_PORT: Database port
#   DB_USER: Database user
#   DB_PASSWORD: Database password
#   DB_NAME: Database name
#   NOT_ENV_MASTER_KEY: Master encryption key (optional, auto-generated if not provided)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=$VERSION" -a -installsuffix cgo -o not-env-backend .

# Runtime stage
FROM alpine:3.22

# Install CA certificates (required for PostgreSQL/MySQL SSL certificate validation)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/not-env-backend .

EXPOSE 1212

CMD ["./not-env-backend"]

