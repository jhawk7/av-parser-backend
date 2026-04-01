# AV Parser Service

A Go-based microservice that ingests media download requests via MQTT, processes them using `yt-dlp` and `ffmpeg`, and tracks job status in Redis.

## Features

- **MQTT Ingestion**: Subscribes to topics for media processing requests.
- **Media Processing**: 
    - Downloads video/audio using `yt-dlp`.
    - Extracts audio to MP3 using `ffmpeg`.
- **Job Tracking**: Uses Redis to store job metadata and status.
- **REST API**: 
    - `GET /health`: Service health check.
    - `GET /jobs`: List all current jobs from Redis.

## Environment Variables

The service uses the following environment variables (defined in `.env` for Docker Compose):

| Variable | Description | Default (in .env) |
|----------|-------------|---------|
| `MQTT_SERVER` | Hostname of the MQTT broker | `host.docker.internal` |
| `MQTT_PORT` | Port of the MQTT broker | `1883` |
| `MQTT_USER` | MQTT username | `admin` |
| `MQTT_PASS` | MQTT password | `admin` |
| `MQTT_TOPICS` | Comma-separated list of topics to subscribe to | `av/requests` |
| `REDIS_HOST` | Host and port for the Redis instance | `redis:6379` |
| `REDIS_PASS` | Password for the Redis instance | `""` |

## Getting Started with Docker

### Prerequisites

- Docker and Docker Compose installed.
- An MQTT broker running (if not using the service on the same host).

### Running the Service

1.  **Configure environment**: Update the `.env` file with your specific MQTT and Redis settings.
2.  **Start with Docker Compose**:
    ```bash
    docker-compose up --build
    ```

The service will be available at `http://localhost:8888`.

### Bind Mounts

The `docker-compose.yml` includes bind mounts for persistent storage of processed files:

- `./audio_archive`: Maps to `/app/audio_archive` in the container.
- `./video_archive`: Maps to `/app/video_archive` in the container.

## MQTT Payload Format

The service expects JSON payloads in the following format on the configured topics:

```json
{
  "url": "https://www.youtube.com/watch?v=...",
  "type": "audio" 
}
```
*`type` can be "audio" or "video".*
