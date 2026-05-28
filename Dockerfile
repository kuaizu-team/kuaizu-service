# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache make git nodejs npm

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Generate code
RUN make generate

ARG VERSION=dev
ARG COMMIT=none

# Build main server
RUN go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date +%F_%T)" -o /app/bin/kuaizu-server cmd/server/main.go

# Build admin server
RUN go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date +%F_%T)" -o /app/bin/kuaizu-admin cmd/admin/main.go

# Run stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies.
# curl is used by Docker Compose healthchecks.
RUN apk add --no-cache ca-certificates tzdata curl

# Copy binaries from builder
COPY --from=builder /app/bin/kuaizu-server /app/kuaizu-server
COPY --from=builder /app/bin/kuaizu-admin /app/kuaizu-admin

# Default port
EXPOSE 8080 8081

# Command will be overridden in docker-compose for each service
CMD ["./kuaizu-server"]
