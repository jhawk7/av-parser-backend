# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# update ytdlp go package
#RUN go get -u github.com/lrstanley/go-ytdlp@latest

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o av-parser-service ./cmd/av-parser-service/main.go


# Final stage
FROM alpine:latest

WORKDIR /app

# Install dependencies: ffmpeg, python3, pip, bun (js runtime for yt-dlp)
RUN apk add --no-cache \
    ffmpeg \
    python3 \
    py3-pip \
    ca-certificates \
    curl \
    bash \
    unzip \
    && curl -fsSL https://bun.sh/install | bash \
    && pip install --no-cache-dir --break-system-packages yt-dlp

RUN yt-dlp -U

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/av-parser-service .

# Create directories for processing and archiving
RUN mkdir -p yt-tmp audio-tmp

# Expose port 8888 to the outside world
EXPOSE 8888

# Command to run the executable
CMD ["./av-parser-service"]
