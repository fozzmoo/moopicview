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

**Testing & TDD:**
- Backend: Go built-in testing framework with test database
- Frontend: Vitest + Testing Library + JSDOM
- Mock APIs for isolated frontend testing
- Test database for backend integration tests

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
photo_tags (photo_id, tag_id, pos_x, pos_y) -- many-to-many with position

comments (id, photo_id, user_id, content, created_at)

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
- **Collections**: Hierarchical navigation through collections
  - Level 1: Collections (from PHOTO_ROOTS: Digital, Scanned)
  - Level 2: Years (derived from directories, e.g., 2017, 2024)
  - Level 3: Event/folder names (e.g., 20170625-FortBuenaVentura, 20240404)
  - Level 4: Photo grid in each folder
- **Search**: By filename, description, date (supports partial dates), tags (full-text where possible)
- **View & Download**: Lightbox viewer with EXIF if available, download button
- **Comment**: Threaded comments per photo
- **Tag**: Add/remove shared global tags to photos with position and visibility controls
  - Tags are stored in a global `tags` table and associated with photos via `photo_tags` junction table
  - Users can add existing tags or create new tags via the tagging dialog
  - Tags are displayed as removable chips on the photo detail page
  - Each tag has a position (X, Y) stored as percentages (0-100) for overlay positioning
  - **Tag Positioning**: Position calculated by clicking on thumbnail preview in tagging dialog
  - **Accurate Coordinate Calculation**: Account for letterboxing in images displayed with `object-contain`
  - **Tag Visibility Toggle**: Tag icon button (top-right of image) to show/hide all tags on image (hidden by default)
  - **Hover-to-Reveal**: Tag labels appear when hovering over tag markers on the image
  - **Tag Search/Browse**: Dedicated page listing all tags with photo counts
  - **Tag Photos Page**: View all photos associated with a specific tag
  - Tags support search and filtering (future enhancement)
- **Propose Edit**: Suggest changes to description, photo date, or date precision; notifies admin
- **Photo Date Management**:
  - Digital photos: Auto-extract from EXIF or directory name (e.g., `20170625-FortBuenaVentura`)
  - Scanned photos: Auto-extract from filename patterns, manual entry for unknown dates
  - Admins can edit dates; users can propose date changes

### Admin Features
- Create new user accounts (First name, Last name, Email, Password [optional], Admin status). If no password is provided, a password reset link is emailed.
- Delete user accounts (prevents deleting the last admin)
- Manage users and pending account requests
- Change user passwords
- Toggle admin privileges for users
- Review/approve/reject proposed edits (triggers email to proposer)
- Direct edit of photo metadata (including date with precision)
- View activity logs
- Trigger manual rescan
- All admin endpoints are protected with role-based access control (requires admin role)

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
  - Metadata display: Collection, date, location
  - Smart date formatting based on precision (June 1989 instead of 1989-06-01)
  - Admin users can edit photo date directly from viewer
  - **Comments**: List of comments from oldest to newest with user name and timestamp, plus form to post new comments (authenticated users only)
  - **Progressive Image Loading**: Thumbnails load instantly, full images fade in when ready
- **Collections Interface**:
  - Card-based collection display with counts
  - Folder grid with hover effects
  - Photo grid with image thumbnails (progressive loading)
  - Search functionality for filtering photos
  - Responsive design for desktop, tablet, and mobile
- **Accessibility**: High contrast ratios, keyboard navigation, proper ARIA labels

### Background Services
- On startup: Scan directories specified in `PHOTO_ROOTS` recursively, upsert into `photos` table
- File system watcher (fsnotify) for new/deleted files
- **Thumbnail Generation**: On-demand generation with caching
  - **Backend Endpoint**: `GET /thumbnails/:id` (served outside `/api` prefix to bypass auth middleware for static asset delivery)
  - **Generation Logic**:
    - Reads photo path from database using ID
    - Opens source image using `imaging` library
    - Resizes to 300px width maintaining aspect ratio (Lanczos filter)
    - Saves to cache directory with `.webp` extension
  - **Caching**:
    - On-demand generation: Generates thumbnail on first request
    - Disk caching: Stores thumbnails permanently at `THUMBNAIL_CACHE_DIR` (default `/opt/mooview/cache`)
    - Instant subsequent access: Serves cached files directly without regeneration
    - Production path: `/unas/images/mooview_cache` on tic
  - **Progressive Loading**:
    - Thumbnail displays immediately
    - Full image fades in when loaded
    - Implemented in `ProgressiveImage` React component

### Hierarchical Collection Navigation

**Navigation Levels:**
1. **Collections** (`GET /api/collections`):
   - Lists all collection types from `PHOTO_ROOTS` (e.g., "Digital", "Scanned")
   - Each shows total photo count
   - UI: Card-based or list view

2. **Subcollections/Folders** (`GET /api/collections/:id`):
   - Returns subdirectories and photos for a specific folder ID
   - If the folder has subdirectories, they are listed
   - If the folder is a leaf (no subdirectories), photos are displayed
   - Derived from database folder structure

**URL Structure:**
- `/collections` → Collections list
- `/collections/{id}` → Subcollections (Years/Folders) or Photos (if leaf folder)

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
- Attempts to extract date from filename using these patterns (in order of specificity):
  - `YYYYMMDD-` → exact date (e.g., `20170625-FortBuenaVentura` → 2017-06-25, exact)
  - `YYYY-MMDD-` → exact date (e.g., `1994-1216-LoganTemple` → 1994-12-16, exact)
  - `YYYY-MM-` → month precision (e.g., `1994-12-ChristineDoran` → 1994-12-01, month)
  - `1989-06-HyrumParty-HeatherRyan.jpg` → June 1989 (month precision)
  - `YYYY-` (with non-digit after) → year precision (e.g., `2019-FamilyVacation` → 2019-01-01, year)
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

**Date Display Formatting:**
- Dates are stored in the database as full dates (e.g., 1989-06-01) with precision metadata
- Frontend formats dates based on precision:
  - `year` → "1989" (only year displayed)
  - `month` → "June 1989" (month name and year, day omitted)
  - `exact` → "June 25, 2017" (full date with month name)
  - `unknown` → "Unknown date"
- This avoids showing misleading dates like "1989-06-01" when only the month is known
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
- `GET /api/collections/:id` (returns subdirectories, photos, and breadcrumbs in given folder ID)

**Photos:**
- `GET /api/photos` (search, pagination, filters)
- `GET /api/photos/:id`
- `GET /api/photos/:id/content` (protected image serve)
- `GET /api/photos/:id/comments` (get comments for a photo)
- `POST /api/photos/:id/comments` (add comment to a photo - requires auth)
- `GET /api/photos/:id/tags` (get tags for a photo)
- `POST /api/photos/:id/tags` (add tag to a photo - requires auth)
- `DELETE /api/photos/:id/tags/:tagId` (remove tag from photo - requires auth)
- `GET /api/tags` (get all available tags for autocomplete)
- `GET /api/tags/:id/photos` (get all photos for a specific tag)
- `POST /api/photos/:id/propose-edit`

**Admin:**
- `GET /api/admin/users` - Requires admin role
- `POST /api/admin/users` - Create new user (first_name, last_name, email, password, is_admin) - Requires admin role
- `POST /api/admin/users/:id/approve` - Requires admin role
- `POST /api/admin/users/:id/change-password` - Change user password - Requires admin role
- `POST /api/admin/users/:id/toggle-admin` - Toggle admin status - Requires admin role
- `GET /api/admin/proposed-edits` - Requires admin role
- `POST /api/admin/proposed-edits/:id/review` - Requires admin role
- `POST /api/admin/photos/:id/date` - Edit photo date (photo_date, date_precision) - Requires admin role

**Protected static routes** for built React app.

**Currently Implemented Endpoints:**
- ✅ `GET /api/health` - Health check
- ✅ `GET /api/auth/login` - Login endpoint
- ✅ `POST /api/auth/change-password` - Change user password
- ✅ `GET /api/collections` - List all collections with photo counts
- ✅ `GET /api/collections/:id` - Get folder contents by ID
- ✅ `GET /api/photos` - List recent photos (paginated)
- ✅ `GET /api/photos/:id` - Get photo metadata (with prev/next navigation IDs and comments)
- ✅ `GET /api/photos/:id/content` - Serve image file
- ✅ `GET /api/photos/:id/comments` - Get comments for a photo
- ✅ `POST /api/photos/:id/comments` - Add comment to a photo (requires auth)
- ✅ `GET /api/photos/:id/tags` - Get tags for a photo
- ✅ `GET /api/tags` - Get all available tags
- ✅ `GET /api/tags/:id/photos` - Get photos for a specific tag
- ✅ `POST /api/photos/:id/tags` - Add tag to a photo (requires auth)
- ✅ `DELETE /api/photos/:id/tags/:tagId` - Remove tag from photo (requires auth)
- ✅ `POST /api/scan` - Trigger photo scan
- ✅ `GET /api/admin/users` - List all users (requires admin role)
- ✅ `POST /api/admin/users` - Create new user account (requires admin role)
- ✅ `POST /api/admin/users/:id/approve` - Approve user account (requires admin role)
- ✅ `POST /api/admin/users/:id/change-password` - Change user password (requires admin role)
- ✅ `POST /api/admin/users/:id/toggle-admin` - Toggle admin status (requires admin role)
- ✅ `DELETE /api/admin/users/:id/delete` - Delete user account (requires admin role)
- ✅ `GET /api/admin/proposed-edits` - List proposed edits (requires admin role)
- ✅ `POST /api/admin/proposed-edits/:id/review` - Review proposed edit (requires admin role)
- ✅ `POST /api/admin/photos/:id/date` - Edit photo date (requires admin role)

**Frontend Features Implemented:**
- ✅ Authentication flow (login, protected routes)
- ✅ Theme switching (light/dark/system)
- ✅ Hierarchical browsing (collections → years → folders → photos)
- ✅ Photo grid with thumbnails
- ✅ Photo viewer with navigation (previous/next buttons, keyboard shortcuts)
- ✅ Download functionality
- ✅ Breadcrumb navigation
- ✅ Search/filter photos
- ✅ Responsive navbar
- ✅ Admin dashboard (user management, proposed edits review)
- ✅ Create new users with admin checkbox
- ✅ Toggle admin status for existing users
- ✅ Delete user with confirmation dialog
- ✅ TDD harness setup (Go tests + Vitest + Testing Library)
- ✅ Tag management (add/remove tags with position)
- ✅ Tag search/browse page (list all tags, search by name)
- ✅ Tag photos page (view all photos for a specific tag)
- ✅ Hover-to-reveal tag labels on photos
  - ✅ Tag visibility toggle (show/hide all tags on photo)
  - ✅ Improved tag positioning (accounts for letterboxing in images)
  - ✅ User account page
- ✅ Previous/next navigation in photo viewer
- ✅ Photo comments (view and post comments with timestamp)

## 8. Frontend Structure

```
src/
├── components/     # UI components
│   ├── ui/         # shadcn/ui components (Button, Card, Badge, Dropdown Menu, etc.)
│   ├── navbar.tsx  # Main navigation bar with theme toggle
│   ├── theme-toggle.tsx  # Dark/light mode switcher
│   ├── theme-provider.tsx  # Theme context provider
│   └── ProgressiveImage.tsx  # Thumbnail-first image loading component
├── pages/          # Page components
│   ├── Login.tsx
│   ├── Collections.tsx  # Collections and photo browsing
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

**Progressive Image Loading Component:**
- **Component**: `ProgressiveImage.tsx`
- **Purpose**: Provides thumbnail-first image loading with smooth fade-in transition
- **Features**:
  - Accepts `src` (full image URL) and `thumbnail` (thumbnail URL) props
  - Displays thumbnail immediately while preloading full image
  - Shows loading placeholder (gray pulse animation) while thumbnail loads
  - Fades in full image when loaded, maintaining aspect ratio
  - Error handling for failed thumbnail or full image loads
- **Usage**:
  - Collections page: Displays photo thumbnails in grid view
  - PhotoView page: Shows high-resolution images with instant thumbnail preview
- **Backend Integration**:
  - Thumbnail URLs point to `/thumbnails/:id` endpoint
  - Full image URLs point to `/api/photos/:id/content` endpoint
  - Thumbnails are generated on-demand and cached permanently

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
- State persists across page transitions (Collections → PhotoView)
- Allows breadcrumb navigation back to any folder level

**PhotoView Component Features:**
- **Image Display**: Uses `ProgressiveImage` component for thumbnail-first loading
- **Tag Markers**: Position markers overlay on the image at stored X/Y coordinates
- **Tag Visibility Toggle**: Button (top-right of image) to show/hide all tags
- **Tag Positioning**: Click thumbnail preview in tagging dialog to set position
- **Edge Navigation**: Click left/right 15% of image to navigate to previous/next photo
- **Tags Section**: List of all tags on the photo in the sidebar
- **Hover Synchronization**: Hovering tag in sidebar highlights marker on image
- **Keyboard Navigation**: Arrow keys for previous/next photo, Escape to exit fullscreen

## 9. Testing & TDD Harness

### Backend Testing (Go)
**Framework:** Go built-in testing package
**Test Database:** `moopicview_test` (isolated from production)

**Test Coverage:**
- `TestLoginHandler` - Tests login with valid/invalid credentials
- `TestAdminUsersHandler` - Tests listing all users
- `TestAdminUserApproveHandler` - Tests approving user accounts
- `TestHealthHandler` - Tests health check endpoint

**Running Tests:**
```bash
TEST_DATABASE_URL="postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable" go test -v ./cmd/server/
```

### Frontend Testing (Vitest + Testing Library)
**Framework:** Vitest + Testing Library + JSDOM
**Setup File:** `src/test/setup.ts` - Configures test environment and mocks

**Test Coverage:**
- `Login.test.tsx` - Login form rendering, error handling, form submission
- `Collections.test.tsx` - Collections list and folder navigation
- `PhotoView.test.tsx` - Photo details, navigation, download functionality
- `AdminDashboard.test.tsx` - User management and proposed edits review
- `Navbar.test.tsx` - Navigation links and theme toggle

**Running Tests:**
```bash
cd frontend && npm run test
```

**Key Testing Practices:**
- Mock API calls using `vi.mock('axios')`
- Wrap components with necessary providers (AuthProvider, BrowserRouter)
- Use `waitFor` for async operations
- Test both success and error scenarios
- Verify user interactions and navigation

### TDD Benefits
- **Early Bug Detection:** Catches issues before they reach production
- **Design Feedback:** Tests drive better component and API design
- **Regression Prevention:** Ensures new features don't break existing functionality
- **Documentation:** Tests serve as living documentation of expected behavior
- **Confidence:** Safe refactoring with test coverage

## 10. Security Considerations

- All file serving protected by auth middleware
- Rate limiting on login and password reset
- JWT expiration + refresh mechanism
- Input sanitization (comments, descriptions) using bluemonday library to prevent XSS
- Parameterized SQL queries to prevent SQL injection
- No storage of sensitive data beyond necessities
- Run container as least-privilege user with read-only access to `/unas` mount
- CSP, XSS protection in React
- Caddy on `lok` provides HTTPS, rate limiting, and optional access controls

## 11. Networking & Infrastructure

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

## 13. API Endpoints

### 13.1 Public Routes
- `GET /api/health` - Server status
- `POST /api/auth/login` - User login
- `POST /api/auth/google` - Google OAuth initiation
- `GET /api/auth/google/callback` - Google OAuth callback
- `POST /api/auth/request-access` - Request account access
- `POST /api/auth/reset-password` - Initiate password reset
- `GET /reset-password` - Password reset page (serves frontend SPA)
- `GET /api/photos` - List photos with filtering
- `GET /api/photos/{id}` - Get photo details with breadcrumbs
- `GET /api/photos/{id}/content` - Serve photo file
- `GET /thumbnails/{id}` - Serve thumbnail (300px width, cached)
- `GET /api/photos/{id}/comments` - Get photo comments
- `GET /api/collections` - List root collections
- `GET /api/collections/{id}` - Get collection contents (folders/photos)
- `GET /api/folders` - List all folders

### 13.2 Authenticated Routes (require JWT)
- `POST /api/photos/{id}/comments` - Add comment to photo

### 13.3 Admin Routes (require admin role)
- `GET /api/admin/users` - List all users
- `POST /api/admin/users` - Create new user
- `POST /api/admin/users/{id}/approve` - Approve user account
- `POST /api/admin/users/{id}/change-password` - Change user password
- `POST /api/admin/users/{id}/toggle-admin` - Toggle admin role
- `DELETE /api/admin/users/{id}/delete` - Delete user
- `POST /api/admin/users/{id}/reset-password` - Reset user password (sends email)
- `GET /api/admin/account-requests` - List account requests
- `POST /api/admin/account-requests/{id}/review` - Review account request
- `GET /api/admin/proposed-edits` - List proposed edits
- `POST /api/admin/proposed-edits/{id}/review` - Review proposed edit
- `POST /api/admin/photos/{id}/date` - Update photo date
- `POST /api/admin/photos/{id}/description` - Update photo description
- `POST /api/scan` - Trigger photo scan (admin only)

### 13.4 Frontend SPA Routes
The backend serves the React SPA for all non-API routes:
- `GET /` - Home page (serves `index.html`)
- `GET /login` - Login page
- `GET /collections` - Collections browse page
- `GET /collections/{id}` - Specific collection page
- `GET /photo/{id}` - Photo detail page
- `GET /account` - User account page
- `GET /admin` - Admin dashboard
- `GET /reset-password` - Password reset page

## 14. Future Enhancements (Not in MVP)

- Face recognition
- AI image description/tagging
- Mobile PWA
- Album/share links
- Bulk operations

---

*This document will evolve as implementation proceeds. Next step: implement database migrations and core Go backend scaffolding.*
