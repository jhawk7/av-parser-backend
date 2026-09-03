#Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

#Copy go mod and sum files
COPY go.mod go.sum ./

#Download all dependencies
RUN go mod download

#update ytdlp go package
RUN go get -u github.com/lrstanley/go-ytdlp@latest

#Copy the source from the current directory
COPY . .

#Build the Go app
RUN go build -o av-parser-service ./cmd/av-parser-service/main.go


#Final stage
FROM alpine:latest

WORKDIR /app

# Install deps. Install deno as JS runtime for yt-dlp (natively supported)
RUN apk add --no-cache \
    ffmpeg \
    python3 \
    py3-pip \
    ca-certificates \
    curl \
    bash \
    unzip \
    deno

#Install and update yt-dlp
RUN pip install --no-cache-dir --break-system-packages yt-dlp
RUN yt-dlp -U

#Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/av-parser-service .

#Create directories for processing and archiving
RUN mkdir -p yt-tmp audio-tmp

#Expose port 8888 to the outside world
EXPOSE 8888

#Command to run the executable
CMD ["./av-parser-service"]
