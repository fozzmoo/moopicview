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
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
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
		rootsStr = "/unas/images"
	}
	roots := strings.Split(rootsStr, ",")
	log.Println("Scanning photos in", roots)

	// Delete missing files
	for _, root := range roots {
		root = strings.TrimSpace(root)
		rows, err := db.Query("SELECT id, filepath FROM photos WHERE filepath LIKE $1 ESCAPE '/'", root+"%")
		if err != nil {
			log.Println("Delete query error for", root, ":", err)
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
	for _, root := range roots {
		root = strings.TrimSpace(root)
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := strings.ToLower(d.Name())
			if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".png") {
				collection := "digital"
				if strings.Contains(path, "scanned_photos") {
					collection = "scanned"
				}
				_, err = db.Exec(`
					INSERT INTO photos (filepath, filename, collection, description)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (filepath) DO UPDATE SET filename = EXCLUDED.filename
				`, path, d.Name(), collection, "Scanned photo")
				if err == nil {
					log.Println("Added/Updated:", d.Name())
				}
			}
			return nil
		})
	}
	log.Println("Scan complete.")
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
		ID          int    `json:"id"`
		Filename    string `json:"filename"`
		Description string `json:"description"`
		Collection  string `json:"collection"`
		ScanDate    string `json:"scan_date"`
		ContentURL  string `json:"content_url"`
	}
	err := db.QueryRow(`
		SELECT id, filename, description, collection, COALESCE(scan_date::text, '')
		FROM photos WHERE id = $1
	`, id).Scan(&photo.ID, &photo.Filename, &photo.Description, &photo.Collection, &photo.ScanDate)
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
