# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o av-parser-service ./cmd/av-parser-service/main.go

# Final stage
FROM alpine:latest

# Install dependencies: ffmpeg, python3, pip, and yt-dlp
RUN apk add --no-cache \
    ffmpeg \
    python3 \
    py3-pip \
    ca-certificates \
    && pip install --no-cache-dir --break-system-packages yt-dlp

WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/av-parser-service .

# Create directories for processing and archiving
RUN mkdir -p yt-tmp audio-tmp audio_archive video_archive

# Expose port 8888 to the outside world
EXPOSE 8888

# Command to run the executable
CMD ["./av-parser-service"]
