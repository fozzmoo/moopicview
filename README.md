# MoopicView

Web application for securely sharing and collaborating on personal photo collections stored on a local Linux server.

See [DESIGN.md](DESIGN.md) for detailed architecture, data model, API design, networking setup (Docker on `tic`, Caddy proxy on `lok`), and implementation plan.

## Features
- Login with email/password or Google
- Account request/approval workflow with admin panel
- Browse, search, view, download, comment, and tag photos
- Shared tags and collaborative metadata editing (with approval)
- Email notifications (via Mailcow)
- Protected access to photos on mounted NAS storage

## Tech Stack
- **Backend**: Go + PostgreSQL
- **Frontend**: React + Vite + TypeScript + TailwindCSS
- **Deployment**: Docker on `tic` (Fedora), Caddy reverse proxy on `lok`, CIFS mount from Ubiquiti NAS at `/unas`

## Quick Start (Local Development)

### Option 1: Docker Compose (Recommended)
1. Clone repo and copy `.env.example` to `.env`
2. `podman-compose -f docker-compose.local.yml up -d`
3. Access via http://localhost:8787

### Option 2: Manual Podman
1. Clone repo and copy `.env.example` to `.env`
2. Build and run database:
   ```bash
   podman run -d --name moopicview_db -p 7432:5432 \
     -e POSTGRES_DB=moopicview \
     -e POSTGRES_USER=moopicview \
     -e POSTGRES_PASSWORD=moopicview123 \
     -v moopicview_postgres_data:/var/lib/postgresql/data \
     postgres:16-alpine
   ```
3. Build and run app:
   ```bash
   CGO_ENABLED=0 go build -o moopicview-server ./cmd/server/
   podman build -t moopicview-local .
   podman run -d --name moopicview --network moopicview_default \
     -p 8787:8080 \
     -v /drv/origVideo/mooview/cache:/mooview_cache \
     -v /drv/origVideo/mooview/digital:/opt/mooview/digital:ro \
     -v /drv/origVideo/mooview/scanned:/opt/mooview/scanned:ro \
     --restart unless-stopped moopicview-local
   ```

## Production Deployment

For production deployment on `tic`, see [DEPLOYMENTS.md](DEPLOYMENTS.md).

## Repository
https://github.com/fozzmoo/moopicview
