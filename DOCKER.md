# Docker Deployment Guide

This document provides comprehensive instructions for deploying Calibre-Web Automated Book Downloader using Docker.

## Overview

The application now uses a **hybrid Go + Python architecture**:
- **Go**: Core application (API server, queue management, downloads, book search)
- **Python**: Cloudflare bypass functionality (temporary until Go implementation is complete)

The Docker image uses a multi-stage build to compile the Go binary and includes minimal Python dependencies for the Cloudflare bypasser.

## Quick Start

### Using Docker Compose (Recommended)

1. **Download the docker-compose.yml file:**
   ```bash
   curl -O https://raw.githubusercontent.com/veverkap/calibre-web-automated-book-downloader/main/docker-compose.yml
   ```

2. **Start the service:**
   ```bash
   docker compose up -d
   ```

3. **Access the web interface:**
   ```
   http://localhost:8084
   ```

### Using Docker CLI

```bash
docker run -d \
  --name calibre-web-automated-book-downloader \
  -p 8084:8084 \
  -v /path/to/ingest:/cwa-book-ingest \
  -e TZ=America/New_York \
  -e UID=1000 \
  -e GID=100 \
  ghcr.io/calibrain/calibre-web-automated-book-downloader:latest
```

## Docker Compose Configuration

### Basic Configuration

```yaml
services:
  calibre-web-automated-book-downloader:
    image: ghcr.io/calibrain/calibre-web-automated-book-downloader:latest
    container_name: calibre-web-automated-book-downloader
    environment:
      FLASK_PORT: 8084
      LOG_LEVEL: info
      BOOK_LANGUAGE: en
      USE_BOOK_TITLE: true
      TZ: America/New_York
      APP_ENV: prod
      UID: 1000
      GID: 100
      MAX_CONCURRENT_DOWNLOADS: 3
    ports:
      - 8084:8084
    restart: unless-stopped
    volumes:
      - /path/to/calibre-web/ingest:/cwa-book-ingest
```

### With Authentication

To enable authentication, mount Calibre-Web's database:

```yaml
services:
  calibre-web-automated-book-downloader:
    image: ghcr.io/calibrain/calibre-web-automated-book-downloader:latest
    container_name: calibre-web-automated-book-downloader
    environment:
      FLASK_PORT: 8084
      CWA_DB_PATH: /auth/app.db
      # ... other environment variables
    ports:
      - 8084:8084
    restart: unless-stopped
    volumes:
      - /path/to/calibre-web/ingest:/cwa-book-ingest
      - /path/to/calibre-web/config/app.db:/auth/app.db:ro
```

**Important:** If your library volume is on a CIFS share, add `nobrl` to your mount options to avoid "database locked" errors:
```bash
//192.168.1.1/Books /media/books cifs credentials=.smbcredentials,uid=1000,gid=1000,iocharset=utf8,nobrl
```

## Environment Variables

### Application Settings

| Variable          | Description             | Default Value      |
| ----------------- | ----------------------- | ------------------ |
| `FLASK_PORT`      | Web interface port      | `8084`             |
| `FLASK_HOST`      | Web interface binding   | `0.0.0.0`          |
| `DEBUG`           | Debug mode toggle       | `false`            |
| `INGEST_DIR`      | Book download directory | `/cwa-book-ingest` |
| `TZ`              | Container timezone      | `UTC`              |
| `UID`             | Runtime user ID         | `1000`             |
| `GID`             | Runtime group ID        | `100`              |
| `CWA_DB_PATH`     | Calibre-Web's database  | None               |
| `ENABLE_LOGGING`  | Enable log file         | `true`             |
| `LOG_LEVEL`       | Log level               | `info`             |

Available log levels: `DEBUG`, `INFO`, `WARNING`, `ERROR`, `CRITICAL`

### Download Settings

| Variable               | Description                                               | Default Value                     |
| ---------------------- | --------------------------------------------------------- | --------------------------------- |
| `MAX_RETRY`            | Maximum retry attempts                                    | `3`                               |
| `DEFAULT_SLEEP`        | Retry delay (seconds)                                     | `5`                               |
| `MAIN_LOOP_SLEEP_TIME` | Processing loop delay (seconds)                           | `5`                               |
| `SUPPORTED_FORMATS`    | Supported book formats                                    | `epub,mobi,azw3,fb2,djvu,cbz,cbr` |
| `BOOK_LANGUAGE`        | Preferred language(s) - comma separated                   | `en`                              |
| `AA_DONATOR_KEY`       | Anna's Archive donator key for fast downloads             | ``                                |
| `USE_BOOK_TITLE`       | Use book title as filename instead of ID                  | `false`                           |
| `PRIORITIZE_WELIB`     | Download from WELIB first instead of AA                   | `false`                           |
| `ALLOW_USE_WELIB`      | Allow usage of welib for downloading                      | `true`                            |
| `MAX_CONCURRENT_DOWNLOADS` | Number of simultaneous downloads                      | `3`                               |

### Anna's Archive Settings

| Variable               | Description                                               | Default Value                     |
| ---------------------- | --------------------------------------------------------- | --------------------------------- |
| `AA_BASE_URL`          | Base URL of Anna's Archive (can use proxy)                | `https://annas-archive.org`       |
| `USE_CF_BYPASS`        | Enable Cloudflare bypass (Python-based)                   | `true`                            |

### Network Settings

| Variable               | Description                     | Default Value           |
| ---------------------- | ------------------------------- | ----------------------- |
| `AA_ADDITIONAL_URLS`   | Proxy URLs for AA (, separated) | ``                      |
| `HTTP_PROXY`           | HTTP proxy URL                  | ``                      |
| `HTTPS_PROXY`          | HTTPS proxy URL                 | ``                      |
| `CUSTOM_DNS`           | Custom DNS IP or preset         | ``                      |
| `USE_DOH`              | Use DNS over HTTPS              | `false`                 |

#### Proxy Configuration

```yaml
environment:
  # Basic proxy
  HTTP_PROXY: http://proxy.example.com:8080
  HTTPS_PROXY: http://proxy.example.com:8080
  
  # Proxy with authentication
  HTTP_PROXY: http://username:password@proxy.example.com:8080
  HTTPS_PROXY: http://username:password@proxy.example.com:8080
```

#### DNS Configuration

**Custom DNS Servers** (e.g., PiHole):
```yaml
environment:
  CUSTOM_DNS: 127.0.0.53,127.0.1.53
```

**Preset DNS Providers**:
```yaml
environment:
  CUSTOM_DNS: cloudflare  # Options: google, quad9, cloudflare, opendns
  USE_DOH: true           # Enable DNS over HTTPS
```

### Custom Script Integration

| Variable               | Description                                                 | Default Value           |
| ---------------------- | ----------------------------------------------------------- | ----------------------- |
| `CUSTOM_SCRIPT`        | Path to script that runs after each download                | ``                      |

Example configuration:
```yaml
environment:
  CUSTOM_SCRIPT: /scripts/process-book.sh

volumes:
  - ./local/scripts/custom_script.sh:/scripts/process-book.sh
```

The script receives the downloaded file path as an argument and must preserve the filename.

## Docker Variants

### Standard Variant (Default)

The standard variant includes the Go application with Python-based Cloudflare bypass.

```bash
docker compose up -d
```

### Tor Variant

Routes all traffic through the Tor network for enhanced privacy.

**Download and start:**
```bash
curl -O https://raw.githubusercontent.com/veverkap/calibre-web-automated-book-downloader/main/docker-compose.tor.yml
docker compose -f docker-compose.tor.yml up -d
```

**Requirements:**
- `NET_ADMIN` and `NET_RAW` capabilities
- Automatic timezone detection based on Tor exit node

**Limitations:**
- Custom DNS, DoH, and proxy settings are ignored
- `TZ` environment variable is overridden by auto-detection

### External Cloudflare Bypass Variant

Uses an external Cloudflare resolver service (e.g., FlareSolverr, ByParr).

**Download and start:**
```bash
curl -O https://raw.githubusercontent.com/veverkap/calibre-web-automated-book-downloader/main/docker-compose.extbp.yml
docker compose -f docker-compose.extbp.yml up -d
```

**Configuration:**
```yaml
environment:
  USE_CF_BYPASS: true
  EXT_BYPASSER_URL: http://flaresolverr:8191
  EXT_BYPASSER_PATH: /v1
  EXT_BYPASSER_TIMEOUT: 60000
```

## Building from Source

### Prerequisites

- Docker 20.10 or later
- Docker Compose v2

### Build the Image

```bash
# Clone the repository
git clone https://github.com/veverkap/calibre-web-automated-book-downloader.git
cd calibre-web-automated-book-downloader

# Build the image
docker build -t calibre-web-automated-book-downloader:local .

# Or use docker compose
docker compose build
```

### Multi-stage Build Process

The Dockerfile uses a multi-stage build:

1. **Stage 1 (go-builder)**: Compiles the Go binary
   - Uses `golang:1.24.9-alpine`
   - Builds static binary with CGO for SQLite support
   
2. **Stage 2 (base)**: Runtime image
   - Uses `python:3.10-slim`
   - Copies Go binary from builder
   - Installs minimal Python dependencies for Cloudflare bypass
   
3. **Stage 3 (cwa-bd)**: Adds browser automation
   - Installs Chromium and ChromeDriver for Cloudflare bypass
   
4. **Stage 4 (cwa-bd-tor)**: Adds Tor support
   - Installs Tor and iptables

## Health Checks

The container includes a health check endpoint:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8084/request/api/status"]
  interval: 30s
  timeout: 30s
  start_period: 5s
  retries: 3
```

Check container health:
```bash
docker ps
docker inspect --format='{{.State.Health.Status}}' calibre-web-automated-book-downloader
```

## Logging

### View Logs

```bash
# View real-time logs
docker logs -f calibre-web-automated-book-downloader

# View last 100 lines
docker logs --tail 100 calibre-web-automated-book-downloader
```

### Log Files

Logs are stored in `/var/log/cwa-book-downloader/` inside the container.

To persist logs, mount a volume:
```yaml
volumes:
  - /path/to/logs:/var/log/cwa-book-downloader
```

## Troubleshooting

### Container Won't Start

1. **Check logs:**
   ```bash
   docker logs calibre-web-automated-book-downloader
   ```

2. **Verify permissions:**
   ```bash
   ls -la /path/to/ingest
   # Should be writable by UID:GID specified in config
   ```

3. **Check port conflicts:**
   ```bash
   netstat -tulpn | grep 8084
   ```

### Database Locked Errors

If using a CIFS share for the library, add `nobrl` to mount options:
```bash
//server/share /mount/point cifs credentials=file,uid=1000,gid=1000,nobrl
```

### Download Failures

1. **Check network connectivity:**
   ```bash
   docker exec calibre-web-automated-book-downloader curl -I https://annas-archive.org
   ```

2. **Try with CF bypass disabled:**
   ```yaml
   environment:
     USE_CF_BYPASS: false
   ```

3. **Use external bypasser:**
   Switch to the external bypasser variant if built-in bypass fails.

### Permission Issues

Ensure UID/GID match your host user:
```yaml
environment:
  UID: 1000  # Use: id -u
  GID: 100   # Use: id -g
```

## Upgrading

### Pull Latest Image

```bash
docker compose pull
docker compose up -d
```

### Backup Before Upgrading

```bash
# Backup your volumes
docker compose down
tar -czf backup-$(date +%Y%m%d).tar.gz /path/to/ingest
docker compose up -d
```

## Performance Tuning

### Concurrent Downloads

Adjust based on your bandwidth and system resources:
```yaml
environment:
  MAX_CONCURRENT_DOWNLOADS: 5  # Default: 3
```

### Memory Limits

Set resource limits to prevent OOM:
```yaml
services:
  calibre-web-automated-book-downloader:
    deploy:
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M
```

## Architecture

### Hybrid Design

The current implementation uses:
- **Go**: 
  - HTTP API server (chi router)
  - Queue management (priority queue)
  - Download system (goroutines for concurrency)
  - Book search and metadata extraction (goquery)
  
- **Python**: 
  - Cloudflare bypass (Selenium + ChromeDriver)
  - Custom DNS/DoH resolution
  
This hybrid approach provides:
- ✅ Better performance (Go's concurrency model)
- ✅ Lower memory footprint (~100MB vs ~250MB Python-only)
- ✅ Faster startup (<100ms vs 1-3s)
- ✅ Reliable Cloudflare bypass (mature Python ecosystem)

### Future Migration

The Cloudflare bypass will eventually be migrated to Go using:
- `chromedp` (Chrome DevTools Protocol)
- Or keeping it as a microservice
- Or using external bypass services (FlareSolverr)

## Security Considerations

### Secrets Management

Never commit sensitive data to docker-compose.yml:

```yaml
# Use environment file
env_file:
  - .env

# Or use Docker secrets
secrets:
  aa_donator_key:
    file: ./secrets/aa_donator_key.txt
```

### Network Isolation

Use Docker networks to isolate services:
```yaml
networks:
  calibre-network:
    driver: bridge

services:
  calibre-web-automated-book-downloader:
    networks:
      - calibre-network
```

### Read-only Filesystem

For enhanced security, mount the app.db as read-only:
```yaml
volumes:
  - /path/to/app.db:/auth/app.db:ro
```

## Support

For issues or questions:
- GitHub Issues: https://github.com/veverkap/calibre-web-automated-book-downloader/issues
- Discussions: https://github.com/veverkap/calibre-web-automated-book-downloader/discussions

## License

See [LICENSE](LICENSE) file for details.
