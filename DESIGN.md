# MoopicView Design Document

## 1. Overview

MoopicView is a secure web application for sharing access to personal photo collections stored on a local Linux server. It provides authenticated browsing, searching, commenting, tagging, and collaborative metadata editing for two primary photo repositories:

- `/unas/images/digital_photos`
- `/unas/images/scanned_photos/` (organized as `YYYY/YYYYMMDD[-description]/IMAGEFILE.jpg`)

**Core Goals:**
- Controlled access via login or approval workflow
- Rich photo discovery and collaboration features
- Admin moderation of users, metadata, and proposed edits
- Email notifications for key events

## 2. Technology Stack

**Backend:**
- Go (API server)
- PostgreSQL (metadata, users, tags, comments, edit proposals)
- File system access to `/unas/images/*`

**Frontend:**
- React + Vite + TypeScript
- TailwindCSS for styling
- React Router for navigation

**Authentication:**
- Email/password (bcrypt)
- Google OAuth2
- JWT tokens stored in http-only cookies
- Account request/approval workflow

**Other:**
- SMTP via Mailcow server (`ion.fozzilinymoo.org`, fozzilinymoo.org domain)
- Background file scanner + watcher
- CLI tool for initial admin setup

## 3. High-Level Architecture

```
External Clients --HTTPS--> lok (Caddy reverse proxy)
                               |
                               +--> tic:8080 (Docker container: Go API + React SPA)
                                         |
                                         +-- PostgreSQL (host or container)
                                         |
                                         +-- /unas/images/... (CIFS mount from Ubiquiti NAS, read-only)
                                         |
                                         +-- SMTP (Mailcow at ion.fozzilinymoo.org)
```

- Go server (in Docker on `tic`) serves both API and built React static files
- Caddy on `lok` handles TLS termination and proxies to the container
- Photos served via protected `/api/photos/content/:id` endpoint with auth middleware
- Background goroutine scans and watches for new photos on startup (fsnotify on mounted volume)

## 4. Data Model (PostgreSQL)

```sql
-- Core tables
users (id, email, password_hash, name, google_id, role, approved, created_at)
account_requests (id, email, name, message, status, reviewed_by, reviewed_at)

photos (
  id,
  filepath,
  filename,
  collection, -- 'digital' or 'scanned'
  scan_date DATE, -- when photo was scanned/imported (less useful for content)
  photo_date DATE, -- actual date photo was taken (nullable, for digital photos from EXIF/directory)
  date_precision VARCHAR(10), -- 'exact', 'month', 'year', 'unknown' (for scanned photos with partial dates)
  date_source VARCHAR(20), -- 'exif', 'directory', 'manual', 'estimated', 'unknown'
  description TEXT,
  original_date TIMESTAMP, -- from EXIF full timestamp
  width INTEGER,
  height INTEGER,
  imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)

tags (id, name) -- global shared tags
photo_tags (photo_id, tag_id) -- many-to-many

comments (id, photo_id, user_id, content, created_at, parent_id)

proposed_edits (
  id, 
  photo_id, 
  user_id, 
  field, -- 'description' or 'date'
  proposed_value, 
  current_value, 
  status, -- pending/approved/rejected
  reviewed_by, reviewed_at,
  created_at
)

-- Audit/log table for actions
activity_logs (id, user_id, action, entity_type, entity_id, details, created_at)
```

**Indexes:** On filepath (unique), scan_date, description (fulltext if possible), tags.

## 5. Authentication & Authorization

- **Login page**: Tabs for "Sign in", "Request Access", "Sign in with Google"
- Email/password users: standard login + "Forgot password" → reset email with time-limited token
- Google OAuth2 callback creates/ links account (pending approval if new)
- All authenticated routes require valid JWT in http-only cookie
- Roles: `user`, `admin`
- First admin created via `go run cmd/setup/admin.go`

**Flows:**
1. New user requests access → email to admins
2. Admin approves → user can login
3. Password reset flow with secure token

## 6. Key Features

### User Features
- **Browse**: Tree or grid view of collections (Digital / Scanned by year/month)
- **Search**: By filename, description, date (supports partial dates), tags (full-text where possible)
- **View & Download**: Lightbox viewer with EXIF if available, download button
- **Comment**: Threaded comments per photo
- **Tag**: Add/remove shared global tags
- **Propose Edit**: Suggest changes to description, photo date, or date precision; notifies admin
- **Photo Date Management**:
  - Digital photos: Auto-extract from EXIF or directory name (e.g., `20170625-FortBuenaVentura`)
  - Scanned photos: Manual entry with precision control (year, year-month, year-month-day, or unknown)
  - Admins can edit dates; users can propose date changes

### Admin Features
- Manage users and pending account requests
- Review/approve/reject proposed edits (triggers email to proposer)
- Direct edit of photo metadata
- View activity logs
- Trigger manual rescan

### Background Services
- On startup: Scan both directories recursively, upsert into `photos` table
- File system watcher (fsnotify) for new/deleted files
- Thumbnail generation (optional, stored alongside or in cache dir)

### Photo Date Handling

**Digital Photos:**
- Extract date from EXIF metadata (preferred)
- Fallback to directory name parsing (e.g., `20170625-FortBuenaVentura` → 2017-06-25)
- Store in `photo_date` with `date_precision='exact'` and `date_source='exif'` or `'directory'`
- `scan_date` set to import time

**Scanned Photos:**
- `scan_date` set to import time (when the physical photo was scanned)
- `photo_date` initially NULL, `date_precision='unknown'`, `date_source='unknown'`
- Admins (or users via proposed edit) can set:
  - Year only → `photo_date=YYYY-01-01`, `date_precision='year'`, `date_source='manual'`
  - Year+Month → `photo_date=YYYY-MM-01`, `date_precision='month'`, `date_source='manual'`
  - Full date → `photo_date=YYYY-MM-DD`, `date_precision='exact'`, `date_source='manual'`
  - Leave unknown → `photo_date=NULL`, `date_precision='unknown'`

**Date Display:**
- Exact: "June 15, 2017"
- Month: "June 2017"
- Year: "2017"
- Unknown: "Unknown date"

## 7. API Design (Go)

**Auth:**
- `POST /api/auth/login`
- `POST /api/auth/google`
- `POST /api/auth/request-access`
- `POST /api/auth/reset-password`

**Photos:**
- `GET /api/photos` (search, pagination, filters)
- `GET /api/photos/:id`
- `GET /api/photos/:id/content` (protected image serve)
- `POST /api/photos/:id/tags`
- `POST /api/photos/:id/comments`
- `POST /api/photos/:id/propose-edit`

**Admin:**
- `GET /api/admin/users`
- `POST /api/admin/users/:id/approve`
- `GET /api/admin/proposed-edits`
- `POST /api/admin/proposed-edits/:id/review`

**Protected static routes** for built React app.

## 8. Frontend Structure

```
src/
├── components/     # Reusable (PhotoGrid, Lightbox, TagInput, CommentThread)
├── pages/          # Login, Browse, PhotoView, AdminDashboard, Profile
├── hooks/          # useAuth, usePhotos, useSearch
├── lib/            # api client, auth utils
├── types/          # TypeScript interfaces matching backend
└── App.tsx
```

- Responsive design optimized for both desktop and tablet
- Infinite scroll or pagination for large collections
- Dark theme by default (photo app aesthetic)

## 9. Security Considerations

- All file serving protected by auth middleware
- Rate limiting on login and password reset
- JWT expiration + refresh mechanism
- Input sanitization (comments, descriptions)
- No storage of sensitive data beyond necessities
- Run container as least-privilege user with read-only access to `/unas` mount
- CSP, XSS protection in React
- Caddy on `lok` provides HTTPS, rate limiting, and optional access controls

## 10. Networking & Infrastructure

**Hosts:**
- **`tic`**: Fedora 43 file server. Runs the MoopicView Docker container. Mounts Ubiquiti NAS via CIFS/SMB at `/unas`.
- **`lok`**: Fedora 43 router/gateway. Runs Caddy as reverse proxy for HTTPS termination and routing to `tic`.
- **Ubiquiti NAS**: Primary storage for photos, exported via CIFS to `tic:/unas`.

**Traffic Flow:**
- Public/ LAN clients → `lok` (Caddy on standard ports 80/443) → reverse proxy to `tic:8080` (or configured port)
- Caddy handles TLS certificates (Let's Encrypt or internal CA), logging, and basic security headers
- Internal DNS should resolve `moopicview.lan` (or similar) to `lok`

**Docker Considerations on `tic`:**
- Container must have access to host's `/unas` mount (use volume mount: `-v /unas:/unas:ro`)
- `fsnotify` for file watching may require `--privileged` or `docker run` with appropriate capabilities
- PostgreSQL can run on host, in separate container, or via Docker Compose
- Use `docker-compose.yml` to manage app container + optional DB

## 12. Deployment & Setup

**On `tic` (Fedora 43):**
1. Install Go: `sudo dnf install -y golang`
2. Ensure CIFS mount of Ubiquiti NAS is active at `/unas` (add to `/etc/fstab` if needed)
3. Setup PostgreSQL: `sudo dnf install -y postgresql-server` and initialize
4. Build Docker image (multi-stage: build React + Go binary)
5. Configure `.env` or Docker secrets:
   - `DATABASE_URL`
   - `JWT_SECRET`
   - `GOOGLE_CLIENT_ID/SECRET`
   - `SMTP_HOST`, `SMTP_USER`, `SMTP_PASS` (Mailcow account on fozzilinymoo.org)
   - `PHOTO_ROOT=/unas/images`
   - `LISTEN_ADDR=:8080`
6. Run initial admin setup command
7. Deploy with `docker compose up -d`

**On `lok`:**
- Install and configure Caddy with a reverse proxy stanza:
  ```
  moopicview.lan {
      reverse_proxy tic:8080
      tls internal
  }
  ```
- (Or use domain with Let's Encrypt if exposed.)

**Background scan** runs automatically on container start. Ensure volume permissions allow reading `/unas`.

## 13. Future Enhancements (Not in MVP)

- Face recognition
- AI image description/tagging
- Mobile PWA
- Album/share links
- Bulk operations

---

*This document will evolve as implementation proceeds. Next step: implement database migrations and core Go backend scaffolding.*
