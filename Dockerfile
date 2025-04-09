FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o shipyard-action .

# Use a smaller base image for the final image
FROM alpine:3.21

RUN apk --no-cache add \
    ca-certificates \
    docker-cli \
    curl \
    bash

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/shipyard-action /app/shipyard-action

# Make the binary executable
RUN chmod +x /app/shipyard-action

# Add metadata labels
LABEL org.opencontainers.image.source="https://github.com/elijahmont3x/shipyard-action" \
      org.opencontainers.image.description="GitHub Action for deploying multi-application setups"

# Set the entrypoint
ENTRYPOINT ["/app/shipyard-action"]