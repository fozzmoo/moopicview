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
- TailwindCSS v4 for styling
- React Router for navigation
- shadcn/ui component library (Button, Card, Badge, Dropdown Menu, etc.)
- Lucide React for icons
- Context API for theme and navigation state management

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
- **Browse**: Hierarchical navigation through collections
  - Level 1: Collections (from PHOTO_ROOTS: Digital, Scanned)
  - Level 2: Years (derived from directories, e.g., 2017, 2024)
  - Level 3: Event/folder names (e.g., 20170625-FortBuenaVentura, 20240404)
  - Level 4: Photo grid in each folder
- **Search**: By filename, description, date (supports partial dates), tags (full-text where possible)
- **View & Download**: Lightbox viewer with EXIF if available, download button
- **Comment**: Threaded comments per photo
- **Tag**: Add/remove shared global tags
- **Propose Edit**: Suggest changes to description, photo date, or date precision; notifies admin
- **Photo Date Management**:
  - Digital photos: Auto-extract from EXIF or directory name (e.g., `20170625-FortBuenaVentura`)
  - Scanned photos: Auto-extract from filename patterns, manual entry for unknown dates
  - Admins can edit dates; users can propose date changes

### Admin Features
- Manage users and pending account requests
- Review/approve/reject proposed edits (triggers email to proposer)
- Direct edit of photo metadata
- View activity logs
- Trigger manual rescan

### UI/UX Features
- **Modern Component Library**: Built with shadcn/ui for consistent, accessible components
- **Theme System**: Dark/light mode toggle with system preference support
  - Light mode: White/light gray backgrounds with dark text
  - Dark mode: Dark gray backgrounds with light text
  - System mode: Automatically follows OS preference
  - Theme preference persisted in localStorage
- **Responsive Navigation**:
  - Fixed navbar with logo, navigation links, and user controls
  - Theme toggle button in top-right corner
  - Breadcrumb navigation showing full path hierarchy
  - Path context for maintaining navigation state across views
- **Photo Viewer Enhancements**:
  - Vertical layout: Image takes full width, info panel below
  - Download functionality with proper filename handling
  - Navigation controls (previous/next) with keyboard shortcuts
  - Action buttons: Add to favorites, Download, Share
  - Metadata display: Collection, date, location, tags
- **Browse Interface**:
  - Card-based collection display with counts
  - Folder grid with hover effects
  - Photo grid with image thumbnails
  - Search functionality for filtering photos
  - Responsive design for desktop, tablet, and mobile
- **Accessibility**: High contrast ratios, keyboard navigation, proper ARIA labels

### Background Services
- On startup: Scan directories specified in `PHOTO_ROOTS` recursively, upsert into `photos` table
- File system watcher (fsnotify) for new/deleted files
- Thumbnail generation (optional, stored alongside or in cache dir)

### Hierarchical Collection Navigation

**Navigation Levels:**
1. **Collections** (`GET /api/collections`):
   - Lists all collection types from `PHOTO_ROOTS` (e.g., "Digital", "Scanned")
   - Each shows total photo count
   - UI: Card-based or list view

2. **Years** (`GET /api/browse?path=/unas/images/digital_photos`):
   - Scans DB for unique year directories under collection path
   - Lists years (2017, 2018, 2024) with photo counts
   - Derived from directory structure: `/YYYY/YYYYMMDD-*`

3. **Event Folders** (`GET /api/browse?path=/unas/images/digital_photos/2017`):
   - Lists event/folder names (20170625-FortBuenaVentura) with photo counts
   - Extracted from YYYYMMDD-* pattern

4. **Photos** (`GET /api/browse?path=/unas/images/digital_photos/2017/20170625-FortBuenaVentura`):
   - Displays photo grid for this specific folder
   - Supports pagination/infinite scroll
   - Clicking opens photo viewer

**URL Structure:**
- `/browse` → Collections list
- `/browse?path=/unas/images/digital_photos` → Years
- `/browse?path=/unas/images/digital_photos/2017` → Folders
- `/browse?path=/unas/images/digital_photos/2017/20170625-FortBuenaVentura` → Photos

### PHOTO_ROOTS Configuration

The `PHOTO_ROOTS` environment variable uses a `type:path` format to specify photo sources:

```
PHOTO_ROOTS=digital:/unas/images/digital_photos/2017/20170625-FortBuenaVentura,scanned:/unas/images/scanned_photos/scan-date/2018/20180726-Slides
```

**Format:** `collection_type:absolute_path` (comma-separated for multiple roots)

**Supported types:**
- `digital`: For digital photos - automatically extracts date from directory names (YYYYMMDD pattern)
- `scanned`: For scanned photos - date is initially unknown, can be manually set later

**Date extraction for digital photos:**
- Scans parent directory name for `YYYYMMDD` pattern
- Example: `20170625-FortBuenaVentura` → photo_date=2017-06-25, date_precision='exact', date_source='directory'

**Date extraction for scanned photos:**
- Attempts to extract date from filename using these patterns (in order):
  - `YYYY-MMDD-` → exact date (e.g., `1994-1216-LoganTemple` → 1994-12-16, exact)
  - `YYYY-MM-` → month precision (e.g., `1994-12-ChristineDoran` → 1994-12-01, month)
  - `YYYY-` (with digit after) → month precision (e.g., `1989-06-HyrumParty` → 1989-06-01, month)
  - `YYYY-` (with non-digit after) → year precision (e.g., `1994-ChristineBridalPhoto` → 1994-01-01, year)
- If no pattern matches, date remains unknown

**Scanned photos:**
- photo_date=NULL, date_precision='unknown', date_source='unknown' on import
- Can be manually edited to year, year-month, or year-month-day with appropriate precision

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

**Collections & Browse (Hierarchical Navigation):**
- `GET /api/collections` (list all collections with photo counts from PHOTO_ROOTS)
- `GET /api/browse?path=/path/to/dir` (returns subdirectories and photos in given path)

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

**Currently Implemented Endpoints:**
- ✅ `GET /api/health` - Health check
- ✅ `GET /api/auth/login` - Login endpoint
- ✅ `GET /api/collections` - List all collections with photo counts
- ✅ `GET /api/browse?path=/path/to/dir` - Browse directory contents
- ✅ `GET /api/photos` - List recent photos (paginated)
- ✅ `GET /api/photos/:id` - Get photo metadata
- ✅ `GET /api/photos/:id/content` - Serve image file
- ✅ `POST /api/scan` - Trigger photo scan
- ✅ `POST /api/auth/change-password` - Change user password

**Frontend Features Implemented:**
- ✅ Authentication flow (login, protected routes)
- ✅ Theme switching (light/dark/system)
- ✅ Hierarchical browsing (collections → years → folders → photos)
- ✅ Photo grid with thumbnails
- ✅ Photo viewer with navigation
- ✅ Download functionality
- ✅ Breadcrumb navigation
- ✅ Search/filter photos
- ✅ Responsive navbar
- ✅ User account page
- ✅ Admin dashboard (basic)

## 8. Frontend Structure

```
src/
├── components/     # UI components
│   ├── ui/         # shadcn/ui components (Button, Card, Badge, Dropdown Menu, etc.)
│   ├── navbar.tsx  # Main navigation bar with theme toggle
│   ├── theme-toggle.tsx  # Dark/light mode switcher
│   └── theme-provider.tsx  # Theme context provider
├── pages/          # Page components
│   ├── Login.tsx
│   ├── Browse.tsx  # Collections and photo browsing
│   ├── PhotoView.tsx  # Individual photo viewer
│   ├── AdminDashboard.tsx
│   └── Account.tsx
├── hooks/          # React hooks
│   └── useAuth.tsx  # Authentication state
├── context/        # React contexts
│   └── PathContext.tsx  # Navigation path state management
├── lib/            # Utilities
│   └── utils.ts    # Helper functions (cn for class merging)
└── App.tsx         # Main app with routing
```

**UI Component System:**
- **shadcn/ui**: Copy-paste components built on Radix UI primitives
- **Tailwind CSS v4**: Modern theming with CSS custom properties
- **Lucide React**: Consistent icon set
- **Path Alias**: `@/` imports for cleaner code

**Theme System:**
- CSS custom properties for color theming (`--color-background`, `--color-foreground`, etc.)
- Theme classes (`dark`, `light`) applied to `<html>` element
- Auto-detects system preference when set to "system" mode
- Seamless transitions between themes with proper contrast ratios

**Navigation State:**
- PathContext maintains navigation breadcrumbs and history
- State persists across page transitions (Browse → PhotoView)
- Allows breadcrumb navigation back to any folder level

- Responsive design optimized for desktop, tablet, and mobile
- Infinite scroll or pagination for large collections
- Dark mode support with high contrast for accessibility

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
   - `CLI_DATABASE_URL` (for localhost development/scans)
   - `JWT_SECRET`
   - `GOOGLE_CLIENT_ID/SECRET`
   - `SMTP_HOST`, `SMTP_USER`, `SMTP_PASS` (Mailcow account on fozzilinymoo.org)
   - `PHOTO_ROOTS=digital:/path/to/digital,scanned:/path/to/scanned` (type:path format)
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
