# MoopicView Deployment Guide

## Local Development

For local testing on this machine, we run the PostgreSQL database and the MoopicView application in Podman containers.

### Prerequisites

- Podman installed and running
- Node.js installed (for frontend builds)
- Go installed (for backend builds)
- PostgreSQL container running (see below)

### Quick Start

**1. Start the database container (if not running):**

```bash
podman start moopicview_db
```

If the container doesn't exist, create it:

```bash
podman run -d --name moopicview_db \
  -p 7432:5432 \
  -e POSTGRES_DB=moopicview \
  -e POSTGRES_USER=moopicview \
  -e POSTGRES_PASSWORD=moopicview123 \
  --network moopicview_default \
  -v moopicview_postgres_data:/var/lib/postgresql/data \
  postgres:16-alpine
```

**2. Build the frontend:**

```bash
cd frontend
npm install  # Only needed once or when dependencies change
npm run build
cd ..
```

**3. Build the backend:**

```bash
# Build a static Go binary (required for Alpine container)
CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/
```

**4. Build the container image:**

```bash
podman build -t localhost/moopicview-local:latest .
```

**5. Deploy the container:**

```bash
# Stop and remove old container if running
podman stop moopicview 2>/dev/null || true
podman rm moopicview 2>/dev/null || true

# Start new container with volume mounts for photos and cache
podman run -d \
  --name moopicview \
  -p 8787:8080 \
  --env-file .env \
  --network moopicview_default \
  -v /drv/origVideo/mooview/cache:/mooview_cache \
  -v /drv/origVideo/mooview/digital:/opt/mooview/digital:ro \
  -v /drv/origVideo/mooview/scanned:/opt/mooview/scanned:ro \
  localhost/moopicview-local:latest
```

**Note**: The volume mounts are required for the server to access photos and generate thumbnails. If thumbnails aren't loading, check that the paths in `.env` (`PHOTO_ROOTS` and `THUMBNAIL_CACHE_DIR`) match the volume mounts.

### Authentication

To access protected routes (like `/photo/{id}`), you must be logged in. The application uses JWT-based authentication.

**To log in:**

1. Navigate to http://localhost:8787/login
2. Enter your admin email and password
3. The JWT token will be stored in localStorage and used for subsequent API requests

**Note**: The thumbnail endpoint (`/thumbnails/{id}`) is public and doesn't require authentication. The full image endpoint (`/api/photos/{id}/content`) and photo view page (`/photo/{id}`) require authentication.

**6. Verify deployment:**

```bash
# Check container logs
podman logs moopicview

# Test the server
curl http://localhost:8787
```

The application will be available at http://localhost:8787

### Development Workflow

For faster development iterations:

**Frontend changes:**
```bash
cd frontend
npm run dev  # Start Vite dev server on port 5173
```

The frontend dev server provides hot-reload and is accessible at http://localhost:5173

**Backend changes:**
```bash
# Rebuild the Go binary
CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/

# Rebuild and restart container
podman build -t localhost/moopicview-local:latest .
podman stop moopicview
podman rm moopicview
podman run -d --name moopicview -p 8787:8080 --env-file .env --network moopicview_default localhost/moopicview-local:latest
```

**One-liner for backend changes:**
```bash
CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/ && \
podman build -t localhost/moopicview-local:latest . && \
podman stop moopicview && podman rm moopicview && \
podman run -d --name moopicview -p 8787:8080 --env-file .env --network moopicview_default localhost/moopicview-local:latest
```

### Troubleshooting

**Database connection errors:**
If you see errors like "dial tcp: lookup moopicview_db: no such host", ensure the database container is running:
```bash
podman start moopicview_db
```

**Container won't start:**
Check the logs:
```bash
podman logs moopicview
```

**Port already in use:**
```bash
# Check what's using port 8787
lsof -i :8787

# Or check for the container
podman ps | grep 8787
```

### Network Configuration

The application and database must be on the same network. The default network is `moopicview_default`:

```bash
# List networks
podman network ls

# Check which containers are on a network
podman network inspect moopicview_default
```

### Cleanup

To remove containers and images:

```bash
# Stop and remove containers
podman stop moopicview moopicview_db
podman rm moopicview moopicview_db

# Remove images (optional)
podman rmi localhost/moopicview-local:latest

# Remove volumes (warning: this deletes database data!)
podman volume rm moopicview_postgres_data
```

## Production Deployment

For "production" deployment, we run both the database and application containers on the host 'tic' in Docker containers and orchestrate the containers with docker compose. The docker-compose.yml file is on tic in the `/home/fozz/work/ephemeraltic` directory.

Tic sits behind a Linux machine called lok which is connected to the Internet. We have Caddy running on lok which proxies traffic to tic on port 8787.

We can reach tic via ssh without a password.

### Building and Deploying to tic

#### Building the container image locally

We build the image on this machine using Podman, then transfer it to tic.

The Go binary **must be statically linked** because the container uses Alpine (musl libc). A dynamically-linked binary will fail with `exec ./moopicview: no such file or directory` at runtime.

```bash
# Build a static Go binary
CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/

# Build the container image (uses the Dockerfile in the repo root)
podman build --no-cache -t moopicview-local .
```

#### Transferring the image to tic

```bash
# Export the image to a tar file
rm -f /tmp/moopicview-local.tar
podman save moopicview-local -o /tmp/moopicview-local.tar

# Copy to tic
scp /tmp/moopicview-local.tar tic:/tmp/moopicview-local.tar
```

#### Deploying on tic

```bash
# Stop and remove the old container, remove the old image
ssh tic "docker kill moopicview; docker rm moopicview; docker rmi moopicview-local:latest localhost/moopicview-local:latest"

# Load the new image and tag it for docker compose
ssh tic "docker load -i /tmp/moopicview-local.tar && docker tag localhost/moopicview-local:latest moopicview-local:latest"

# Start the container
ssh tic "cd /home/fozz/work/ephemeraltic && docker compose up -d moopicview"

# Clean up
ssh tic "rm /tmp/moopicview-local.tar"
rm /tmp/moopicview-local.tar
```

The `docker tag` step is needed because `docker compose` looks for `moopicview-local` (without the `localhost/` prefix that `docker load` creates). Without it, compose will try to pull from a registry and fail.