# AGENTS

## Core Principles (Always Follow)
- **Strict TDD Workflow**: Never implement features without tests first.
  1. Write/update failing tests (Red).
  2. Write minimal code to make tests pass (Green).
  3. Refactor while keeping all tests green.
- Always run tests before proposing changes. Exercise the full test suite alongside the application codebase.
- If tests don't exist for a change, create them first.
- Use tests to validate **all** code changes. Do not assume correctness.

## Testing
- Tests live alongside application code (e.g., in the same directory or standard `__tests__`/test folders).
- Run backend tests with: `TEST_DATABASE_URL="postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable" go test -v ./cmd/server/`
- Run frontend tests with: `cd frontend && npm test -- --run`
- Always verify with `git status` and test runs after changes.
- Prioritize unit + integration tests over assumptions.

## Deployment & Environment Rules (Critical - Never Mix)
- **Local Development**:
  - Use Podman (not Docker unless explicitly requested).
  - Environment: `.env` or dev-specific vars. Use local URLs/ports (e.g., localhost:3000, dev DB).
  - Maintain and use a docker-compose.local.yml file that is included in
    .gitignore
  - Commands: `podman compose up --build` (or your exact local setup).
- **Production**:
  - Separate config. Use production env vars only (no local values).
  - Different deployment method if applicable (e.g., Docker)
  - Never hardcode or leak dev values into prod configs.
- Always specify **which environment** you're targeting in every response/task. Ask for clarification if ambiguous.
- Differentiate Docker vs. Podman explicitly—default to Docker for this project unless stated.
- Refer to DEPLOYMENTS.md for more details

## General Rules
- Maintain full awareness of existing tests and run them.
- Reference this AGENTS.md in every major task summary.
- Be explicit about env differences and container runtime.
- If unsure about context, re-read relevant files/tests before acting.
- Output format: Always include a "Changes Summary" with test commands to run and env notes.

This project uses Go for the backend API. Follow semantic versioning, etc.

The DESIGN.md file should describe all aspects of the project and should be
updated as major design aspects are modified or added.

## Build & Run

### Build Backend
```bash
# Static build for Alpine containers
CGO_ENABLED=0 go build -o bin/moopicview-server ./cmd/server/main.go
```

### Build Frontend
```bash
cd frontend && npm run build
```

### Run Server Locally
Requires a running PostgreSQL database.

```bash
# Set environment variables
export DATABASE_URL="postgres://user:password@localhost/dbname"
export PHOTO_ROOTS="digital:/path/to/photos,scanned:/path/to/scanned"
export LISTEN_ADDR=":8787"
export THUMBNAIL_CACHE_DIR="/tmp/moopicview_cache"

# Run the server
./bin/moopicview-server
```

### Run in Podman Container
1. Build the image:
   ```bash
   podman build -t moopicview:latest .
   ```
2. Run the container (mounts photos and cache, maps port 8787):
   ```bash
   podman run -d --name moopicview \
     -p 8787:8080 \
     -v /path/to/photos:/drv/origVideo/mooview/digital:ro \
     -v /path/to/cache:/drv/origVideo/mooview/cache:ro \
     -e DATABASE_URL="postgres://user:password@host/dbname" \
     moopicview:latest
   ```
   *Note: The container runs on port 8080 internally, mapped to 8787 externally.*

## Key Environment Variables
- `DATABASE_URL`: PostgreSQL connection string.
- `PHOTO_ROOTS`: Comma-separated list of `type:path` roots (e.g., `digital:/photos,scanned:/scans`).
- `LISTEN_ADDR`: Port to listen on (default `:8080`).
- `THUMBNAIL_CACHE_DIR`: Directory for generated thumbnails.
- `FRONTEND_DIST`: Path to frontend build assets (default `../../frontend/dist`).
- `CLI_DATABASE_URL`: Used when running `./moopicview-server scan`.

## Database Management
- **Scan Photos**: Run `./moopicview-server scan` (requires `DATABASE_URL` or `CLI_DATABASE_URL`).
- **Schema**: See `init-db.sql`.
