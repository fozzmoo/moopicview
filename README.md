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

## Quick Start

1. Clone repo and copy `.env.example` to `.env`
2. `docker compose up -d`
3. Run admin setup command to create first user
4. Access via Caddy proxy on `lok`

Full setup and development instructions are in **DESIGN.md**.

## Repository
https://github.com/fozzmoo/moopicview
