package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/rwcarlsen/goexif/exif"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var cliMode = false
var jwtSecret = []byte("supersecret123changeinprod")

type Claims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

var getDBURL = func() string {
	if cliMode {
		dbURL := os.Getenv("CLI_DATABASE_URL")
		if dbURL != "" {
			return dbURL
		}
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		return dbURL
	}
	if cliMode {
		return "postgres://moopicview:moopicview123@localhost:5432/moopicview?sslmode=disable"
	}
	return "postgres://moopicview:moopicview123@db:5432/moopicview?sslmode=disable"
}

// isAdminMiddleware checks if the requesting user is an admin
func isAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the JWT token from the Authorization header
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")

		// Parse the token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			http.Error(w, "Unauthorized: Invalid token claims", http.StatusUnauthorized)
			return
		}

		// Check if user is admin
		db, err := sql.Open("postgres", getDBURL())
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		var role string
		err = db.QueryRow("SELECT role FROM users WHERE email = $1", claims.Email).Scan(&role)
		if err != nil {
			http.Error(w, "Unauthorized: User not found", http.StatusUnauthorized)
			return
		}

		if role != "admin" {
			http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	godotenv.Load()

	if len(os.Args) > 1 && os.Args[1] == "scan" {
		cliMode = true
		scanPhotos()
		return
	}

	port := os.Getenv("LISTEN_ADDR")
	if port == "" {
		port = ":8080"
	}

	r := mux.NewRouter()

	// API routes (registered before catch-all)
	r.HandleFunc("/api/auth/login", loginHandler).Methods("POST")
	r.HandleFunc("/api/auth/change-password", changePasswordHandler).Methods("POST")
	r.HandleFunc("/api/photos", photosHandler).Methods("GET")
	r.HandleFunc("/api/photos/{id}", photoHandler).Methods("GET")
	r.HandleFunc("/api/photos/{id}/content", photoContentHandler).Methods("GET")
	r.HandleFunc("/api/collections", collectionsHandler).Methods("GET")
	r.HandleFunc("/api/collections/{id}", collectionHandler).Methods("GET")
	r.HandleFunc("/api/folders", foldersHandler).Methods("GET")
	r.HandleFunc("/api/scan", scanHandler).Methods("POST")
	r.HandleFunc("/api/health", healthHandler).Methods("GET")

	// Admin routes (protected by admin middleware)
	adminRouter := r.PathPrefix("/api/admin").Subrouter()
	adminRouter.Use(isAdminMiddleware)
	adminRouter.HandleFunc("/users", adminUsersHandler).Methods("GET")
	adminRouter.HandleFunc("/users", adminCreateUserHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/approve", adminUserApproveHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/change-password", adminUserChangePasswordHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/toggle-admin", adminUserToggleAdminHandler).Methods("POST")
	adminRouter.HandleFunc("/proposed-edits", adminProposedEditsHandler).Methods("GET")
	adminRouter.HandleFunc("/proposed-edits/{id}/review", adminProposedEditReviewHandler).Methods("POST")
	adminRouter.HandleFunc("/photos/{id}/date", adminPhotoDateHandler).Methods("POST")
	adminRouter.HandleFunc("/photos/{id}/description", adminPhotoDescriptionHandler).Methods("POST")

	// Serve React SPA
	r.PathPrefix("/").HandlerFunc(spaHandler)

	log.Printf("Starting MoopicView server on %s", port)
	go scanPhotos()

	c := cron.New()
	c.AddFunc("@daily", scanPhotos)
	c.Start()
	defer c.Stop()

	log.Fatal(http.ListenAndServe(port, r))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("MoopicView is running"))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var user struct {
		ID           int
		Email        string
		PasswordHash string
		Role         string
		Approved     bool
	}
	err = db.QueryRow("SELECT id, email, password_hash, role, approved FROM users WHERE email = $1", creds.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Approved)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if !user.Approved {
		http.Error(w, "Account not approved", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(creds.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Email: user.Email,
		Role:  user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	})
	tokenString, _ := token.SignedString(jwtSecret)

	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

func spaHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") {
		http.NotFound(w, r)
		return
	}
	// Handle all routes that should serve index.html
	if r.URL.Path == "/" || r.URL.Path == "/login" || 
	   r.URL.Path == "/collections" || strings.HasPrefix(r.URL.Path, "/collections/") ||
	   strings.HasPrefix(r.URL.Path, "/photo") || 
	   r.URL.Path == "/account" || r.URL.Path == "/admin" {
		http.ServeFile(w, r, "../../frontend/dist/index.html")
		return
	}
	http.FileServer(http.Dir("../../frontend/dist")).ServeHTTP(w, r)
}

func collectionsHandler(w http.ResponseWriter, r *http.Request) {
	rootsStr := os.Getenv("PHOTO_ROOTS")
	if rootsStr == "" {
		rootsStr = "digital:/unas/images/digital_photos/2017/20170625-FortBuenaVentura,scanned:/unas/images/scanned_photos/scan-date/2024/20240404"
	}
	rootEntries := strings.Split(rootsStr, ",")

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	collections := make([]map[string]interface{}, 0)
	for _, entry := range rootEntries {
		entry = strings.TrimSpace(entry)
		parts := strings.SplitN(entry, ":", 2)
		collectionType := "digital"
		path := ""
		if len(parts) == 2 {
			collectionType = strings.TrimSpace(parts[0])
			path = strings.TrimSpace(parts[1])
		} else {
			path = strings.TrimSpace(parts[0])
		}

		// Get the folder ID for this path
		var folderID int
		var name string
		err := db.QueryRow(`SELECT id, name FROM folders WHERE path = $1`, path).Scan(&folderID, &name)
		if err != nil {
			// Folder not found, skip
			continue
		}

		// Count photos in this folder and its subfolders
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM photos WHERE folder_id IN (
				SELECT id FROM folders WHERE path LIKE $1 OR path = $1
			)
		`, path+"%").Scan(&count)
		if err != nil {
			count = 0
		}

		collections = append(collections, map[string]interface{}{
			"id":   folderID,
			"path": path,
			"name": name,
			"type": collectionType,
			"count": count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(collections)
}



func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	claims := token.Claims.(*Claims)
	email := claims.Email

	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://moopicview:moopicview123@localhost:5432/moopicview?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var storedHash string
	err = db.QueryRow("SELECT password_hash FROM users WHERE email = $1", email).Scan(&storedHash)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.OldPassword)); err != nil {
		http.Error(w, "Incorrect current password", http.StatusUnauthorized)
		return
	}

	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	_, err = db.Exec("UPDATE users SET password_hash = $1 WHERE email = $2", newHash, email)
	if err != nil {
		http.Error(w, "Update failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "password updated"})
}

func scanPhotos() {
	dbURL := getDBURL()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Println("Scan DB error:", err)
		return
	}
	defer db.Close()

	rootsStr := os.Getenv("PHOTO_ROOTS")
	if rootsStr == "" {
		rootsStr = "digital:/unas/images"
	}
	rootEntries := strings.Split(rootsStr, ",")
	var rootPaths []string
	for _, entry := range rootEntries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			rootPaths = append(rootPaths, entry)
		}
	}
	log.Println("Scanning photos in", rootPaths)

	// Delete missing files
	for _, entry := range rootPaths {
		parts := strings.SplitN(entry, ":", 2)
		path := ""
		if len(parts) == 2 {
			path = parts[1]
		} else {
			path = parts[0]
		}
		path = strings.TrimSpace(path)

		rows, err := db.Query("SELECT id, filepath FROM photos WHERE filepath LIKE $1 ESCAPE '/'", path+"%")
		if err != nil {
			log.Println("Delete query error for", path, ":", err)
			continue
		}
		for rows.Next() {
			var id int
			var path string
			rows.Scan(&id, &path)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				db.Exec("DELETE FROM photos WHERE id = $1", id)
				log.Println("Deleted:", path)
			}
		}
		rows.Close()
	}

	// Add/update files
	for _, entry := range rootPaths {
		parts := strings.SplitN(entry, ":", 2)
		photoType := "digital"
		path := ""
		if len(parts) == 2 {
			photoType = strings.TrimSpace(parts[0])
			path = strings.TrimSpace(parts[1])
		} else {
			path = strings.TrimSpace(parts[0])
		}

		filepath.WalkDir(path, func(fullPath string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			nameLower := strings.ToLower(name)
			if strings.HasSuffix(nameLower, ".jpg") || strings.HasSuffix(nameLower, ".jpeg") || strings.HasSuffix(nameLower, ".png") {
				// Determine photo date based on type
				var photoDate sql.NullString
				var datePrecision string = "unknown"
				var dateSource string = "unknown"

				if photoType == "digital" {
					// Primary: EXIF date
					if date, precision, ok := extractExifDate(fullPath); ok {
						photoDate = sql.NullString{String: date.Format("2006-01-02"), Valid: true}
						datePrecision = precision
						dateSource = "exif"
					} else {
						// Fallback: directory name
						parentDir := filepath.Base(filepath.Dir(fullPath))
						if date, precision, source, ok := extractDateFromDirName(parentDir); ok {
							photoDate = sql.NullString{String: date.Format("2006-01-02"), Valid: true}
							datePrecision = precision
							dateSource = source
						}
					}
				} else if photoType == "scanned" {
					// For scanned photos, try to extract date from filename
					if date, precision, source, ok := extractDateFromDirName(name); ok {
						photoDate = sql.NullString{String: date.Format("2006-01-02"), Valid: true}
						datePrecision = precision
						dateSource = source
					}
				}

				_, err = db.Exec(`
					INSERT INTO photos (filepath, filename, collection, scan_date, photo_date, date_precision, date_source, description)
					VALUES ($1, $2, $3, CURRENT_DATE, $4, $5, $6, $7)
					ON CONFLICT (filepath) DO UPDATE SET
						filename = EXCLUDED.filename,
						scan_date = CURRENT_DATE
				`, fullPath, name, photoType, photoDate, datePrecision, dateSource, "Scanned photo")
				if err == nil {
					log.Printf("Added/Updated: %s (type=%s, date=%v, precision=%s, source=%s)", name, photoType, photoDate.String, datePrecision, dateSource)
				} else {
					log.Printf("Error inserting %s: %v", fullPath, err)
				}
			}
			return nil
		})
	}
	log.Println("Scan complete.")
}

func extractExifDate(filePath string) (time.Time, string, bool) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, "", false
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, "", false
	}

	dateTime, err := x.DateTime()
	if err != nil {
		return time.Time{}, "", false
	}
	return dateTime, "exact", true
}

func extractDateFromDirName(dirName string) (time.Time, string, string, bool) {
	// Try to match YYYYMMDD pattern at start (directory names, e.g., 20170625-FortBuenaVentura)
	// Check this first as it's the most specific
	re := regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})`)
	matches := re.FindStringSubmatch(dirName)
	if len(matches) == 4 {
		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), "exact", "directory", true
		}
	}

	// Try to match YYYY-MMDD pattern (e.g., 1994-1216-LoganTemple)
	// This is more specific than YYYY-MM-
	re2 := regexp.MustCompile(`^(\d{4})-(\d{2})(\d{2})`)
	matches2 := re2.FindStringSubmatch(dirName)
	if len(matches2) == 4 {
		year, _ := strconv.Atoi(matches2[1])
		month, _ := strconv.Atoi(matches2[2])
		day, _ := strconv.Atoi(matches2[3])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), "exact", "filename", true
		}
	}

	// Try to match YYYY-MM- pattern (e.g., 1994-12-ChristineDoran, 1989-06-HyrumParty)
	// This is more specific than YYYY-
	re3 := regexp.MustCompile(`^(\d{4})-(\d{2})-`)
	matches3 := re3.FindStringSubmatch(dirName)
	if len(matches3) == 3 {
		year, _ := strconv.Atoi(matches3[1])
		month, _ := strconv.Atoi(matches3[2])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 {
			return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), "month", "filename", true
		}
	}

	// Try to match YYYY- pattern (e.g., 2019-FamilyVacation)
	// This is the least specific and should be checked last
	re4 := regexp.MustCompile(`^(\d{4})-[^0-9]`)
	matches4 := re4.FindStringSubmatch(dirName)
	if len(matches4) == 2 {
		year, _ := strconv.Atoi(matches4[1])
		if year >= 1900 && year <= 2100 {
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), "year", "filename", true
		}
	}

	return time.Time{}, "unknown", "unknown", false
}

func photosHandler(w http.ResponseWriter, r *http.Request) {
	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, filename, description, collection FROM photos ORDER BY id DESC LIMIT 50")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var photos []map[string]interface{}
	for rows.Next() {
		var id int
		var filename, description, collection string
		rows.Scan(&id, &filename, &description, &collection)
		photos = append(photos, map[string]interface{}{
			"id":          id,
			"filename":    filename,
			"description": description,
			"collection":  collection,
			"url":         fmt.Sprintf("/api/photos/%d/content", id),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photos)
}

func photoContentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	var filepathStr string
	err := db.QueryRow("SELECT filepath FROM photos WHERE id = $1", id).Scan(&filepathStr)
	if err != nil {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filepathStr)
	if err != nil {
		http.Error(w, "File error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if strings.HasSuffix(strings.ToLower(filepathStr), ".png") {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}

	io.Copy(w, file)
}

func photoHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

   	var photo struct {
   		ID             int      `json:"id"`
   		Filepath       string   `json:"filepath"`
   		Filename       string   `json:"filename"`
   		FolderID       *int     `json:"folder_id"`
   		FolderName     *string  `json:"folder_name"`
   		Description    string   `json:"description"`
   		Collection     string   `json:"collection"`
   		PhotoDate      *string  `json:"photo_date"`
   		DatePrecision  string   `json:"date_precision"`
   		DateSource     string   `json:"date_source"`
   		ContentURL     string   `json:"content_url"`
   		PrevPhotoID    *int     `json:"prev_photo_id"`
   		NextPhotoID    *int     `json:"next_photo_id"`
   	}
   	err := db.QueryRow(`
   		SELECT p.id, p.filepath, p.filename, p.folder_id, f.name, p.description, p.collection, p.photo_date::text, p.date_precision, p.date_source
   		FROM photos p
   		LEFT JOIN folders f ON p.folder_id = f.id
   		WHERE p.id = $1
   	`, id).Scan(&photo.ID, &photo.Filepath, &photo.Filename, &photo.FolderID, &photo.FolderName, &photo.Description, &photo.Collection, &photo.PhotoDate, &photo.DatePrecision, &photo.DateSource)
   	if err != nil {
   		http.Error(w, "Photo not found", http.StatusNotFound)
   		return
   	}
  	photo.ContentURL = fmt.Sprintf("/api/photos/%d/content", photo.ID)

	// Get previous and next photos in the same folder
	if photo.FolderID != nil {
		// Get previous photo (highest ID less than current)
		err = db.QueryRow(`
			SELECT id FROM photos 
			WHERE folder_id = $1 AND id < $2 
			ORDER BY id DESC 
			LIMIT 1
		`, *photo.FolderID, id).Scan(&photo.PrevPhotoID)
		if err != nil {
			photo.PrevPhotoID = nil // No previous photo
		}

		// Get next photo (lowest ID greater than current)
		err = db.QueryRow(`
			SELECT id FROM photos 
			WHERE folder_id = $1 AND id > $2 
			ORDER BY id ASC 
			LIMIT 1
		`, *photo.FolderID, id).Scan(&photo.NextPhotoID)
		if err != nil {
			photo.NextPhotoID = nil // No next photo
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photo)
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	scanPhotos()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scan complete"})
}

// foldersHandler returns all folders for a given collection type or parent folder
func foldersHandler(w http.ResponseWriter, r *http.Request) {
	collectionType := r.URL.Query().Get("type")
	parentID := r.URL.Query().Get("parent")

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	var rows *sql.Rows
	var err error

	if collectionType != "" {
		// Get folders by collection type
		rows, err = db.Query(`
			SELECT id, path, name, parent_path, collection_type
			FROM folders
			WHERE collection_type = $1
			ORDER BY name
		`, collectionType)
	} else if parentID != "" {
		// Get subfolders of a parent folder
		var parentPath string
		err := db.QueryRow(`SELECT path FROM folders WHERE id = $1`, parentID).Scan(&parentPath)
		if err != nil {
			http.Error(w, "Parent folder not found", http.StatusNotFound)
			return
		}
		rows, err = db.Query(`
			SELECT id, path, name, parent_path, collection_type
			FROM folders
			WHERE parent_path = $1
			ORDER BY name
		`, parentPath)
	} else {
		// Get all root folders (no parent)
		rows, err = db.Query(`
			SELECT id, path, name, parent_path, collection_type
			FROM folders
			WHERE parent_path IS NULL OR parent_path = ''
			ORDER BY name
		`)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	folders := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int
		var path, name, parentPath, collectionType string
		rows.Scan(&id, &path, &name, &parentPath, &collectionType)
		folders = append(folders, map[string]interface{}{
			"id":              id,
			"path":            path,
			"name":            name,
			"parent_path":     parentPath,
			"collection_type": collectionType,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
}

// collectionHandler returns a specific collection/folder by ID
func collectionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	// Get folder info
	var folder struct {
		ID              int    `json:"id"`
		Path            string `json:"path"`
		Name            string `json:"name"`
		CollectionType  string `json:"collection_type"`
	}
	err := db.QueryRow(`
		SELECT id, path, name, collection_type
		FROM folders WHERE id = $1
	`, id).Scan(&folder.ID, &folder.Path, &folder.Name, &folder.CollectionType)
	if err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	// Get subdirectories
	rows, err := db.Query(`
		SELECT id, path, name, collection_type
		FROM folders
		WHERE parent_path = $1
		ORDER BY name
	`, folder.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	directories := make([]map[string]interface{}, 0)
	for rows.Next() {
		var dirID int
		var dirPath, dirName, dirCollection string
		rows.Scan(&dirID, &dirPath, &dirName, &dirCollection)
		directories = append(directories, map[string]interface{}{
			"id":   dirID,
			"path": dirPath,
			"name": dirName,
			"type": dirCollection,
		})
	}

	// Get photos in this folder
	photoRows, err := db.Query(`
		SELECT id, filepath, filename, collection, photo_date::text, date_precision
		FROM photos
		WHERE folder_id = $1
		ORDER BY filename
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer photoRows.Close()

	photos := make([]map[string]interface{}, 0)
	for photoRows.Next() {
		var photoID int
		var filepath, filename, collection, photoDate, datePrecision string
		photoRows.Scan(&photoID, &filepath, &filename, &collection, &photoDate, &datePrecision)
		photos = append(photos, map[string]interface{}{
			"id":             photoID,
			"filename":       filename,
			"collection":     collection,
			"photo_date":     photoDate,
			"date_precision": datePrecision,
			"url":            fmt.Sprintf("/api/photos/%d/content", photoID),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"folder":       folder,
		"directories":  directories,
		"photos":       photos,
	})
}

// Admin handlers
func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, email, name, role, approved, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int
		var email, name, role string
		var approved bool
		var createdAt time.Time
		rows.Scan(&id, &email, &name, &role, &approved, &createdAt)
		users = append(users, map[string]interface{}{
			"id":         id,
			"email":      email,
			"name":       name,
			"role":       role,
			"approved":   approved,
			"created_at": createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func adminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		IsAdmin   bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FirstName == "" || req.LastName == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "First name, last name, email, and password are required", http.StatusBadRequest)
		return
	}

	// Validate password length
	if len(req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Determine role
	role := "user"
	if req.IsAdmin {
		role = "admin"
	}

	// Create the user
	fullName := req.FirstName + " " + req.LastName
	_, err = db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, $3, $4, true)
	`, req.Email, string(hashedPassword), fullName, role)
	if err != nil {
		// Check if email already exists
		if strings.Contains(err.Error(), "duplicate key value") {
			http.Error(w, "Email already exists", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "user created"})
}

func adminUserApproveHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	_, err = db.Exec("UPDATE users SET approved = TRUE WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

func adminUserChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	var req struct {
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if len(req.NewPassword) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Update the user's password
	_, err = db.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(hashedPassword), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password updated"})
}

func adminProposedEditsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT pe.id, pe.photo_id, pe.user_id, u.email, pe.field, pe.proposed_value, pe.current_value, pe.status, pe.created_at
		FROM proposed_edits pe
		JOIN users u ON pe.user_id = u.id
		ORDER BY pe.created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	edits := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, photoID, userID int
		var email, field, proposedValue, currentValue, status string
		var createdAt time.Time
		rows.Scan(&id, &photoID, &userID, &email, &field, &proposedValue, &currentValue, &status, &createdAt)
		edits = append(edits, map[string]interface{}{
			"id":             id,
			"photo_id":       photoID,
			"user_id":        userID,
			"user_email":     email,
			"field":          field,
			"proposed_value": proposedValue,
			"current_value":  currentValue,
			"status":         status,
			"created_at":     createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(edits)
}

func adminProposedEditReviewHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Get the proposed edit
	var photoID int
	var field, proposedValue string
	err = db.QueryRow("SELECT photo_id, field, proposed_value FROM proposed_edits WHERE id = $1", id).Scan(&photoID, &field, &proposedValue)
	if err != nil {
		http.Error(w, "Proposed edit not found", http.StatusNotFound)
		return
	}

	// If approved, update the photo
	if req.Status == "approved" {
		var updateQuery string
		if field == "description" {
			updateQuery = "UPDATE photos SET description = $1 WHERE id = $2"
		} else if field == "date" {
			updateQuery = "UPDATE photos SET photo_date = $1::date, date_precision = 'exact', date_source = 'manual' WHERE id = $2"
		} else {
			http.Error(w, "Invalid field", http.StatusBadRequest)
			return
		}
		_, err = db.Exec(updateQuery, proposedValue, photoID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update the proposed edit status
	_, err = db.Exec("UPDATE proposed_edits SET status = $1 WHERE id = $2", req.Status, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func adminUserToggleAdminHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Get current role
	var currentRole string
	err = db.QueryRow("SELECT role FROM users WHERE id = $1", id).Scan(&currentRole)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Toggle role
	var newRole string
	if currentRole == "admin" {
		newRole = "user"
	} else {
		newRole = "admin"
	}

	_, err = db.Exec("UPDATE users SET role = $1 WHERE id = $2", newRole, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "role updated", "new_role": newRole})
}

func adminPhotoDateHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	var req struct {
		PhotoDate     string `json:"photo_date"`
		DatePrecision string `json:"date_precision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// If photo_date is empty or null, set to NULL with unknown precision
	if req.PhotoDate == "" || req.PhotoDate == "unknown" {
		db, err := sql.Open("postgres", getDBURL())
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		_, err = db.Exec(`
			UPDATE photos 
			SET photo_date = NULL, date_precision = 'unknown', date_source = 'manual'
			WHERE id = $1
		`, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "date updated"})
		return
	}

	// Validate precision for non-empty dates
	if req.DatePrecision != "year" && req.DatePrecision != "month" && req.DatePrecision != "exact" {
		http.Error(w, "Invalid date precision. Must be 'year', 'month', or 'exact'", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Update the photo date and precision
	_, err = db.Exec(`
		UPDATE photos 
		SET photo_date = $1::date, date_precision = $2, date_source = 'manual'
		WHERE id = $3
	`, req.PhotoDate, req.DatePrecision, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "date updated"})
}

func adminPhotoDescriptionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Update the photo description
	_, err = db.Exec(`
		UPDATE photos 
		SET description = $1
		WHERE id = $2
	`, req.Description, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "description updated"})
}
