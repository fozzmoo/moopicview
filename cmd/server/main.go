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
	"sort"
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

func getDBURL() string {
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
	r.HandleFunc("/api/browse", browseHandler).Methods("GET")
	r.HandleFunc("/api/scan", scanHandler).Methods("POST")
	r.HandleFunc("/api/health", healthHandler).Methods("GET")

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

	// TODO: replace with real DB check
	if creds.Email == "admin@fozzilinymoo.org" {
		if err := bcrypt.CompareHashAndPassword([]byte("$2a$10$dummyhashfornow"), []byte(creds.Password)); err == nil || creds.Password == "admin123" {
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
				Email: creds.Email,
				Role:  "admin",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				},
			})
			tokenString, _ := token.SignedString(jwtSecret)

			json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
			return
		}
	}

	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
}

func spaHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/" || r.URL.Path == "/login" || r.URL.Path == "/browse" || strings.HasPrefix(r.URL.Path, "/photo") || r.URL.Path == "/account" {
		http.ServeFile(w, r, "frontend/dist/index.html")
		return
	}
	http.FileServer(http.Dir("frontend/dist")).ServeHTTP(w, r)
}

func collectionsHandler(w http.ResponseWriter, r *http.Request) {
	rootsStr := os.Getenv("PHOTO_ROOTS")
	if rootsStr == "" {
		rootsStr = "digital:/unas/images"
	}
	rootEntries := strings.Split(rootsStr, ",")

	var collections []map[string]interface{}
	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

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

		// Count photos in this collection using the path as prefix
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM photos WHERE filepath LIKE $1", path+"%").Scan(&count)
		if err != nil {
			log.Printf("Count query error for %s: %v", path, err)
			count = 0
		}



		// Extract the root collection name from path for display
		pathParts := strings.Split(path, "/")
		displayName := ""
		if len(pathParts) > 0 {
			displayName = pathParts[len(pathParts)-1]
		} else {
			displayName = collectionType
		}

		collections = append(collections, map[string]interface{}{
			"type":  collectionType,
			"path": path,
			"name": displayName,
			"count": count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(collections)
}

func browseHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter required", http.StatusBadRequest)
		return
	}

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	// Find all photos in this directory (direct children only, not subdirectories)
	rows, err := db.Query(`
		SELECT id, filepath, filename, collection, photo_date::text, date_precision
		FROM photos WHERE filepath LIKE $1
		ORDER BY filename
	`, path+"%")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	photos := make([]map[string]interface{}, 0)
	directoriesSet := make(map[string]bool)

	for rows.Next() {
		var id int
		var filepathStr, filename, collection, photoDate, datePrecision string
		rows.Scan(&id, &filepathStr, &filename, &collection, &photoDate, &datePrecision)

		// Extract the directory name immediately following the base path
		relativePath := strings.TrimPrefix(filepathStr, path)
		relativePath = strings.TrimLeft(relativePath, "/")
		pathParts := strings.Split(relativePath, "/")

		// First part after base path is a subdirectory
		if len(pathParts) > 1 {
			directoriesSet[pathParts[0]] = true
		} else if len(pathParts) == 1 {
			// This is a direct child photo
			photos = append(photos, map[string]interface{}{
				"id":             id,
				"filename":       filename,
				"collection":     collection,
				"photo_date":     photoDate,
				"date_precision": datePrecision,
				"url":            fmt.Sprintf("/api/photos/%d/content", id),
			})
		}
	}
	rows.Close()

	// Convert set to sorted slice
	directories := make([]map[string]interface{}, 0)
	for dir := range directoriesSet {
		directories = append(directories, map[string]interface{}{
			"name":  dir,
			"path":  filepath.Join(path, dir),
			"type":  "directory",
		})
	}

	// Sort directories alphabetically
	sort.Slice(directories, func(i, j int) bool {
		return directories[i]["name"].(string) < directories[j]["name"].(string)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"directories": directories,
		"photos":      photos,
		"currentPath": path,
	})
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
	// Try to match YYYY-MMDD pattern (e.g., 1994-1216-LoganTemple)
	re := regexp.MustCompile(`^(\d{4})-(\d{2})(\d{2})`)
	matches := re.FindStringSubmatch(dirName)
	if len(matches) == 4 {
		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), "exact", "filename", true
		}
	}

	// Try to match YYYY-MM- pattern (e.g., 1994-12-ChristineDoran)
	re2 := regexp.MustCompile(`^(\d{4})-(\d{2})-`)
	matches2 := re2.FindStringSubmatch(dirName)
	if len(matches2) == 3 {
		year, _ := strconv.Atoi(matches2[1])
		month, _ := strconv.Atoi(matches2[2])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 {
			return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), "month", "filename", true
		}
	}

	// Try to match YYYY- pattern (e.g., 1989-06-HyrumParty)
	re3 := regexp.MustCompile(`^(\d{4})-[^0-9]`)
	matches3 := re3.FindStringSubmatch(dirName)
	if len(matches3) == 2 {
		year, _ := strconv.Atoi(matches3[1])
		if year >= 1900 && year <= 2100 {
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), "year", "filename", true
		}
	}

	// Try to match YYYYMMDD pattern at start (directory names, e.g., 20170625-FortBuenaVentura)
	re4 := regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})`)
	matches4 := re4.FindStringSubmatch(dirName)
	if len(matches4) == 4 {
		year, _ := strconv.Atoi(matches4[1])
		month, _ := strconv.Atoi(matches4[2])
		day, _ := strconv.Atoi(matches4[3])
		if year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), "exact", "directory", true
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
		ID             int     `json:"id"`
		Filename       string  `json:"filename"`
		Description    string  `json:"description"`
		Collection     string  `json:"collection"`
		PhotoDate      *string `json:"photo_date"`
		DatePrecision  string  `json:"date_precision"`
		DateSource     string  `json:"date_source"`
		ContentURL     string  `json:"content_url"`
	}
	err := db.QueryRow(`
		SELECT id, filename, description, collection, photo_date::text, date_precision, date_source
		FROM photos WHERE id = $1
	`, id).Scan(&photo.ID, &photo.Filename, &photo.Description, &photo.Collection, &photo.PhotoDate, &photo.DatePrecision, &photo.DateSource)
	if err != nil {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}
	photo.ContentURL = fmt.Sprintf("/api/photos/%d/content", photo.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photo)
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	scanPhotos()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scan complete"})
}
