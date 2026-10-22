# Build stage
FROM golang:1.26.10-alpine AS builder

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0

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
RUN go build -trimpath -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date +%F_%T)" -o /app/bin/kuaizu-server ./cmd/server

# Build admin server
RUN go build -trimpath -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date +%F_%T)" -o /app/bin/kuaizu-admin ./cmd/admin

# Run stage
FROM alpine:latest

ARG VERSION=dev
ARG COMMIT=none
LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.revision=${COMMIT}

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
