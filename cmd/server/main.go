package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/microcosm-cc/bluemonday"
	"github.com/robfig/cron/v3"
	"github.com/rwcarlsen/goexif/exif"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var cliMode = false
var jwtSecret = []byte("supersecret123changeinprod")
var p = bluemonday.UGCPolicy()

// sanitizeHTML sanitizes user input to prevent XSS and SQL injection
func sanitizeHTML(input string) string {
	// Use bluemonday to sanitize HTML
	sanitized := p.Sanitize(input)
	
	// Also trim whitespace and limit length
	sanitized = strings.TrimSpace(sanitized)
	if len(sanitized) > 10000 {
		sanitized = sanitized[:10000]
	}
	
	return sanitized
}

// OAuth2 configuration
var oauthConfig *oauth2.Config

func initOAuth() {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	
	if clientID == "" || clientID == "your-google-oauth-client-id" {
		log.Printf("Warning: GOOGLE_CLIENT_ID not set, Google OAuth will not work. Set it in .env file.")
		return
	}
	if clientSecret == "" || clientSecret == "your-google-oauth-client-secret" {
		log.Printf("Warning: GOOGLE_CLIENT_SECRET not set, Google OAuth will not work. Set it in .env file.")
		return
	}

	// Determine the redirect URL based on environment
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")
	if redirectURL == "" {
		// Try to determine from other env vars or use localhost default
		listenAddr := os.Getenv("LISTEN_ADDR")
		if listenAddr != "" && listenAddr != ":8080" {
			// Extract host from LISTEN_ADDR
			host := strings.Split(listenAddr, ":")[0]
			if host != "" {
				redirectURL = fmt.Sprintf("http://%s:8080/api/auth/google/callback", host)
			} else {
				redirectURL = "http://localhost:8080/api/auth/google/callback"
			}
		} else {
			redirectURL = "http://localhost:8080/api/auth/google/callback"
		}
	}

	oauthConfig = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
	
	log.Printf("Google OAuth configured with redirect URL: %s", redirectURL)
}

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

// authMiddleware checks if the requesting user is authenticated (any role)
func authMiddleware(next http.Handler) http.Handler {
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

		// Get user ID from database
		db, err := sql.Open("postgres", getDBURL())
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		var userID int
		err = db.QueryRow("SELECT id FROM users WHERE email = $1", claims.Email).Scan(&userID)
		if err != nil {
			http.Error(w, "Unauthorized: User not found", http.StatusUnauthorized)
			return
		}

		// Add user ID to context
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func main() {

	godotenv.Load()

	// Initialize OAuth configuration
	initOAuth()

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
	r.HandleFunc("/api/auth/google", googleAuthHandler).Methods("GET")
	r.HandleFunc("/api/auth/google/callback", googleAuthCallbackHandler).Methods("GET")
	r.HandleFunc("/api/auth/request-access", requestAccessHandler).Methods("POST")
	r.HandleFunc("/api/auth/change-password", changePasswordHandler).Methods("POST")
	r.HandleFunc("/api/auth/reset-password", passwordResetHandler).Methods("POST")
	r.HandleFunc("/reset-password", passwordResetHandler).Methods("GET")
	r.HandleFunc("/api/photos", photosHandler).Methods("GET")
	r.HandleFunc("/api/photos/{id}", photoHandler).Methods("GET")
	r.HandleFunc("/api/photos/{id}/content", photoContentHandler).Methods("GET")
	r.HandleFunc("/thumbnails/{id}", photoThumbnailHandler).Methods("GET")
	r.HandleFunc("/api/photos/{id}/comments", photoCommentsHandler).Methods("GET")
	r.HandleFunc("/api/collections", collectionsHandler).Methods("GET")
	r.HandleFunc("/api/collections/{id}", collectionHandler).Methods("GET")
	r.HandleFunc("/api/folders", foldersHandler).Methods("GET")
	r.HandleFunc("/api/scan", scanHandler).Methods("POST")
	r.HandleFunc("/api/health", healthHandler).Methods("GET")

	// Authenticated routes (protected by auth middleware)
	authRouter := r.PathPrefix("/api").Subrouter()
	authRouter.Use(authMiddleware)
	authRouter.HandleFunc("/photos/{id}/comments", addPhotoCommentHandler).Methods("POST")

	// Admin routes (protected by admin middleware)
	adminRouter := r.PathPrefix("/api/admin").Subrouter()
	adminRouter.Use(isAdminMiddleware)
	adminRouter.HandleFunc("/users", adminUsersHandler).Methods("GET")
	adminRouter.HandleFunc("/users", adminCreateUserHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/approve", adminUserApproveHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/change-password", adminUserChangePasswordHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/toggle-admin", adminUserToggleAdminHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id}/delete", adminUserDeleteHandler).Methods("DELETE")
	adminRouter.HandleFunc("/users/{id}/reset-password", adminUserResetPasswordHandler).Methods("POST")
	adminRouter.HandleFunc("/account-requests", adminAccountRequestsHandler).Methods("GET")
	adminRouter.HandleFunc("/account-requests/{id}/review", adminAccountRequestReviewHandler).Methods("POST")
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

func googleAuthHandler(w http.ResponseWriter, r *http.Request) {
	if oauthConfig == nil {
		http.Error(w, "Google OAuth not configured", http.StatusInternalServerError)
		return
	}

	// Generate a random state token for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	state := fmt.Sprintf("%x", stateBytes)

	// Store state in a cookie (short-lived)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   false, // Set to true in production
	})

	url := oauthConfig.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func googleAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if oauthConfig == nil {
		http.Error(w, "Google OAuth not configured", http.StatusInternalServerError)
		return
	}

	// Verify state token
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value != state {
		http.Error(w, "Invalid state token", http.StatusBadRequest)
		return
	}

	// Exchange authorization code for tokens
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No authorization code provided", http.StatusBadRequest)
		return
	}

	token, err := oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusBadRequest)
		return
	}

	// Use the token to get user info
	client := oauthConfig.Client(r.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user info: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var userInfo struct {
		Email         string `json:"email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		http.Error(w, "Failed to parse user info", http.StatusInternalServerError)
		return
	}

	if !userInfo.VerifiedEmail {
		http.Error(w, "Email not verified", http.StatusBadRequest)
		return
	}

	// Check if user already exists
	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	var existingUser struct {
		ID       int
		Approved bool
		Role     string
	}

	err = db.QueryRow("SELECT id, approved, role FROM users WHERE email = $1", userInfo.Email).Scan(
		&existingUser.ID, &existingUser.Approved, &existingUser.Role)

	if err != nil {
		// User doesn't exist, create a new user
		_, err = db.Exec(`
			INSERT INTO users (email, name, role, approved, created_at)
			VALUES ($1, $2, 'user', false, CURRENT_TIMESTAMP)
			ON CONFLICT (email) DO NOTHING
		`, userInfo.Email, userInfo.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get the newly created user
		err = db.QueryRow("SELECT id, approved, role FROM users WHERE email = $1", userInfo.Email).Scan(
			&existingUser.ID, &existingUser.Approved, &existingUser.Role)
		if err != nil {
			http.Error(w, "Failed to get user after creation", http.StatusInternalServerError)
			return
		}
	}

	// Check if user is approved
	if !existingUser.Approved {
		http.Error(w, "Account not approved. Please wait for admin approval.", http.StatusUnauthorized)
		return
	}

	// Create JWT token
	tokenStr := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Email: userInfo.Email,
		Role:  existingUser.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	})
	tokenString, _ := tokenStr.SignedString(jwtSecret)

	// Set JWT in http-only cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Path:     "/",
		MaxAge:   24 * 60 * 60, // 24 hours
		HttpOnly: true,
		Secure:   false, // Set to true in production
	})

	// Redirect to collections page
	http.Redirect(w, r, "/collections", http.StatusFound)
}

// sendEmail sends an email using SMTP with SSL/TLS
func sendEmail(to, subject, body string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	if smtpHost == "" || smtpPort == "" || smtpUser == "" || smtpPass == "" {
		log.Printf("Warning: SMTP configuration incomplete. Cannot send email to %s", to)
		return fmt.Errorf("SMTP configuration incomplete")
	}

	// Parse port
	port, err := strconv.Atoi(smtpPort)
	if err != nil {
		return fmt.Errorf("invalid SMTP port: %v", err)
	}

	// Construct email message
	from := smtpUser
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s", from, to, subject, body)

	// Connect to SMTP server with TLS
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	tlsConfig := &tls.Config{
		ServerName: smtpHost,
	}

	// For port 465, we typically use implicit TLS
	// For port 587, we use STARTTLS
	var conn net.Conn
	var err2 error

	if port == 465 {
		conn, err2 = tls.Dial("tcp", fmt.Sprintf("%s:%d", smtpHost, port), tlsConfig)
	} else {
		// For other ports (like 587), we'd use STARTTLS
		// For simplicity, we'll try direct TLS for 465, and fail for others in this basic impl
		return fmt.Errorf("only port 465 (SSL/TLS) is currently supported in this implementation")
	}

	if err2 != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err2)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %v", err)
	}
	defer client.Close()

	// Authenticate
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %v", err)
	}

	// Set sender and recipient
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %v", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %v", err)
	}

	// Send data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %v", err)
	}
	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}
	w.Close()

	log.Printf("Email sent successfully to %s", to)
	return nil
}

func spaHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/thumbnails") {
		http.NotFound(w, r)
		return
	}

	// Determine frontend dist path
	frontendDist := os.Getenv("FRONTEND_DIST")
	if frontendDist == "" {
		// Default to relative path for local development
		frontendDist = "../../frontend/dist"
	}

	// Handle all routes that should serve index.html
	if r.URL.Path == "/" || r.URL.Path == "/login" ||
		r.URL.Path == "/collections" || strings.HasPrefix(r.URL.Path, "/collections/") ||
		strings.HasPrefix(r.URL.Path, "/photo") ||
		r.URL.Path == "/account" || r.URL.Path == "/admin" ||
		r.URL.Path == "/reset-password" {
		http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
		return
	}
	http.FileServer(http.Dir(frontendDist)).ServeHTTP(w, r)
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

		// Get the folder ID for this path, inserting if necessary
		var folderID int
		var name string
		err := db.QueryRow(`
			INSERT INTO folders (path, name, parent_path, collection_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (path) DO UPDATE SET name = EXCLUDED.name
			RETURNING id, name
		`, path, filepath.Base(path), filepath.Dir(path), collectionType).Scan(&folderID, &name)
		if err != nil {
			log.Printf("Error getting/inserting folder %s: %v", path, err)
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



func requestAccessHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Email == "" || req.Name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Check if user already exists
	var existingUser int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", req.Email).Scan(&existingUser)
	if err == nil {
		http.Error(w, "Email already registered", http.StatusBadRequest)
		return
	}

	// Check if account request already exists
	var existingRequest int
	err = db.QueryRow("SELECT id FROM account_requests WHERE email = $1", req.Email).Scan(&existingRequest)
	if err == nil {
		http.Error(w, "Account request already submitted", http.StatusBadRequest)
		return
	}

	// Create account request
	_, err = db.Exec(`
		INSERT INTO account_requests (email, name, message, status)
		VALUES ($1, $2, $3, 'pending')
	`, req.Email, req.Name, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: Send email to admins notifying them of the new request

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "request submitted"})
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

				// Determine folder path and ID
				dirPath := filepath.Dir(fullPath)
				
				// First, ensure the immediate parent folder exists and get its ID
				var folderID int
				err = db.QueryRow(`
					INSERT INTO folders (path, name, parent_path, collection_type)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (path) DO UPDATE SET name = EXCLUDED.name
					RETURNING id
				`, dirPath, filepath.Base(dirPath), filepath.Dir(dirPath), photoType).Scan(&folderID)
				if err != nil {
					log.Printf("Error inserting folder %s: %v", dirPath, err)
					return nil
				}
				
				// Insert all parent directories recursively (for folder navigation)
				currentPath := filepath.Dir(dirPath)
				for {
					if currentPath == "/" || currentPath == "." || currentPath == path {
						break
					}
					parentPath := filepath.Dir(currentPath)
					
					// Insert parent folder (ignore return value)
					_, err = db.Exec(`
						INSERT INTO folders (path, name, parent_path, collection_type)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (path) DO UPDATE SET name = EXCLUDED.name
					`, currentPath, filepath.Base(currentPath), parentPath, photoType)
					if err != nil {
						log.Printf("Error inserting parent folder %s: %v", currentPath, err)
					}
					
					currentPath = parentPath
				}

				_, err = db.Exec(`
					INSERT INTO photos (filepath, filename, collection, folder_id, scan_date, photo_date, date_precision, date_source, description)
					VALUES ($1, $2, $3, $4, CURRENT_DATE, $5, $6, $7, $8)
					ON CONFLICT (filepath) DO UPDATE SET
						filename = EXCLUDED.filename,
						folder_id = EXCLUDED.folder_id,
						scan_date = CURRENT_DATE
				`, fullPath, name, photoType, folderID, photoDate, datePrecision, dateSource, "Scanned photo")
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

func photoThumbnailHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)
	
	log.Printf("Thumbnail request for photo ID: %d", id)

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	var filepathStr string
	err := db.QueryRow("SELECT filepath FROM photos WHERE id = $1", id).Scan(&filepathStr)
	if err != nil {
		log.Printf("Photo not found for ID %d: %v", id, err)
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}
	
	log.Printf("Found photo path: %s", filepathStr)

	// Determine cache directory
	cacheDir := os.Getenv("THUMBNAIL_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/opt/mooview/cache"
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		http.Error(w, "Cache directory error", http.StatusInternalServerError)
		return
	}

	// Generate cache filename (replace path separators with underscores)
	cacheFilename := strings.ReplaceAll(strings.TrimPrefix(filepathStr, "/"), "/", "_")
	cachePath := filepath.Join(cacheDir, cacheFilename+".jpg")

	// Check if thumbnail already exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		// Thumbnail doesn't exist, generate it
		img, err := imaging.Open(filepathStr)
		if err != nil {
			http.Error(w, "Failed to open image", http.StatusInternalServerError)
			return
		}

		// Resize to 300px width, maintain aspect ratio (height = 0 for auto)
		thumbnail := imaging.Resize(img, 300, 0, imaging.Lanczos)

		// Save thumbnail to cache
		err = imaging.Save(thumbnail, cachePath)
		if err != nil {
			http.Error(w, "Failed to save thumbnail", http.StatusInternalServerError)
			return
		}
	}

	// Serve the thumbnail
	file, err := os.Open(cachePath)
	if err != nil {
		http.Error(w, "File error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	
	// Handle HEAD requests (return only headers, no body)
	if r.Method == "HEAD" {
		return
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

	// Get breadcrumbs (parent folders) for the photo's folder
	breadcrumbs := []map[string]interface{}{
		{"id": 0, "name": "Collections", "path": ""},
	}
	if photo.FolderID != nil {
		// Get the folder path first
		var folderPath string
		err = db.QueryRow("SELECT path FROM folders WHERE id = $1", *photo.FolderID).Scan(&folderPath)
		if err == nil {
			// Build breadcrumb path from root to current
			currentPath := folderPath
			parentPaths := []map[string]interface{}{}
			
			for {
				parentPath := filepath.Dir(currentPath)
				if parentPath == "/" || parentPath == "." || parentPath == currentPath {
					break
				}
				
				var parentFolder struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
					Path string `json:"path"`
				}
				err := db.QueryRow(`
					SELECT id, name, path
					FROM folders
					WHERE path = $1
				`, parentPath).Scan(&parentFolder.ID, &parentFolder.Name, &parentFolder.Path)
				if err != nil {
					// Parent folder not found
					break
				}
				
				parentPaths = append([]map[string]interface{}{
					{"id": parentFolder.ID, "name": parentFolder.Name, "path": parentFolder.Path},
				}, parentPaths...)
				
				currentPath = parentPath
			}
			
			// Add parent paths to breadcrumbs
			breadcrumbs = append(breadcrumbs, parentPaths...)
			
			// Add current folder (the photo's folder) at the end
			var photoFolder struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Path string `json:"path"`
			}
			err = db.QueryRow("SELECT id, name, path FROM folders WHERE id = $1", *photo.FolderID).Scan(&photoFolder.ID, &photoFolder.Name, &photoFolder.Path)
			if err == nil {
				breadcrumbs = append(breadcrumbs, map[string]interface{}{
					"id":   photoFolder.ID,
					"name": photoFolder.Name,
					"path": photoFolder.Path,
				})
			}
		}
	}

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

	// Get comments for this photo
	rows, err := db.Query(`
		SELECT c.id, c.content, c.created_at, u.name, u.id
		FROM comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.photo_id = $1
		ORDER BY c.created_at ASC
	`, id)
	if err != nil {
		http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	comments := []map[string]interface{}{}
	for rows.Next() {
		var comment struct {
			ID        int       `json:"id"`
			Content   string    `json:"content"`
			CreatedAt time.Time `json:"created_at"`
			UserName  string    `json:"user_name"`
			UserID    int       `json:"user_id"`
		}
		err := rows.Scan(&comment.ID, &comment.Content, &comment.CreatedAt, &comment.UserName, &comment.UserID)
		if err != nil {
			continue
		}
		comments = append(comments, map[string]interface{}{
			"id":         comment.ID,
			"content":    comment.Content,
			"created_at": comment.CreatedAt,
			"user_name":  comment.UserName,
			"user_id":    comment.UserID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"photo":       photo,
		"breadcrumbs": breadcrumbs,
		"comments":    comments,
	})
}

// photoCommentsHandler returns all comments for a photo
func photoCommentsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	// Check if photo exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM photos WHERE id = $1)", id).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT c.id, c.content, c.created_at, u.name, u.id
		FROM comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.photo_id = $1
		ORDER BY c.created_at ASC
	`, id)
	if err != nil {
		http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	comments := []map[string]interface{}{}
	for rows.Next() {
		var comment struct {
			ID        int       `json:"id"`
			Content   string    `json:"content"`
			CreatedAt time.Time `json:"created_at"`
			UserName  string    `json:"user_name"`
			UserID    int       `json:"user_id"`
		}
		err := rows.Scan(&comment.ID, &comment.Content, &comment.CreatedAt, &comment.UserName, &comment.UserID)
		if err != nil {
			continue
		}
		comments = append(comments, map[string]interface{}{
			"id":         comment.ID,
			"content":    comment.Content,
			"created_at": comment.CreatedAt,
			"user_name":  comment.UserName,
			"user_id":    comment.UserID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// addPhotoCommentHandler adds a new comment to a photo
func addPhotoCommentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	photoID, _ := strconv.Atoi(idStr)

	// Get user from context (requires authentication)
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sanitize content
	sanitizedContent := sanitizeHTML(req.Content)
	if sanitizedContent == "" {
		http.Error(w, "Comment content is required", http.StatusBadRequest)
		return
	}

	// Check if photo exists
	db, _ := sql.Open("postgres", getDBURL())
	defer db.Close()

	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM photos WHERE id = $1)", photoID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	// Insert comment
	var commentID int
	err = db.QueryRow(`
		INSERT INTO comments (photo_id, user_id, content)
		VALUES ($1, $2, $3)
		RETURNING id
	`, photoID, userID, sanitizedContent).Scan(&commentID)
	if err != nil {
		http.Error(w, "Failed to add comment", http.StatusInternalServerError)
		return
	}

	// Get the comment with user info
	var comment struct {
		ID        int       `json:"id"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UserName  string    `json:"user_name"`
		UserID    int       `json:"user_id"`
	}
	err = db.QueryRow(`
		SELECT c.id, c.content, c.created_at, u.name, u.id
		FROM comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.id = $1
	`, commentID).Scan(&comment.ID, &comment.Content, &comment.CreatedAt, &comment.UserName, &comment.UserID)
	if err != nil {
		http.Error(w, "Failed to fetch comment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         comment.ID,
		"content":    comment.Content,
		"created_at": comment.CreatedAt,
		"user_name":  comment.UserName,
		"user_id":    comment.UserID,
	})
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

	// Get breadcrumbs (parent folders)
	// Start with an empty list and build from root to current
	breadcrumbs := []map[string]interface{}{
		{"id": 0, "name": "Collections", "path": ""},
	}
	
	// Get the root collection for this folder
	// The root collection is the first folder in the path hierarchy
	// For example, /opt/mooview/scanned/2026/20260118-DadsDragRacing
	// The root collection is "scanned" at /opt/mooview/scanned
	currentPath := folder.Path
	var pathParts []string
	
	// Split path into parts
	for {
		if currentPath == "/" || currentPath == "." {
			break
		}
		pathParts = append([]string{filepath.Base(currentPath)}, pathParts...)
		currentPath = filepath.Dir(currentPath)
	}
	
	// Build breadcrumb path from root to current
	// For /opt/mooview/scanned/2026/20260118-DadsDragRacing
	// We want: Collections -> scanned -> 2026 -> 20260118-DadsDragRacing
	// But we need to find the root collection first
	// The root collection is the one in PHOTO_ROOTS (e.g., /opt/mooview/scanned)
	// We need to find which folder in the path is a root collection
	
	// For now, let's just traverse up from the current folder and find all parent folders
	// that exist in the database
	currentPath = folder.Path
	parentPaths := []map[string]interface{}{}
	
	for {
		parentPath := filepath.Dir(currentPath)
		if parentPath == "/" || parentPath == "." || parentPath == currentPath {
			break
		}
		
		var parentFolder struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Path string `json:"path"`
		}
		err := db.QueryRow(`
			SELECT id, name, path
			FROM folders
			WHERE path = $1
		`, parentPath).Scan(&parentFolder.ID, &parentFolder.Name, &parentFolder.Path)
		if err != nil {
			// Parent folder not found
			break
		}
		
		parentPaths = append([]map[string]interface{}{
			{"id": parentFolder.ID, "name": parentFolder.Name, "path": parentFolder.Path},
		}, parentPaths...)
		
		currentPath = parentPath
	}
	
	// Add parent paths to breadcrumbs
	breadcrumbs = append(breadcrumbs, parentPaths...)
	
	// Add current folder at the end
	breadcrumbs = append(breadcrumbs, map[string]interface{}{
		"id":   folder.ID,
		"name": folder.Name,
		"path": folder.Path,
	})

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

	// Get photos in this folder only (not subfolders)
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
		"breadcrumbs":  breadcrumbs,
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
		FirstName string  `json:"first_name"`
		LastName  string  `json:"last_name"`
		Email     string  `json:"email"`
		Password  *string `json:"password"` // Pointer to distinguish empty string from null
		IsAdmin   bool    `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FirstName == "" || req.LastName == "" || req.Email == "" {
		http.Error(w, "First name, last name, and email are required", http.StatusBadRequest)
		return
	}

	// If password is provided, validate it
	if req.Password != nil && len(*req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Determine role
	role := "user"
	if req.IsAdmin {
		role = "admin"
	}

	// Create the user
	fullName := req.FirstName + " " + req.LastName
	
	// If password is provided, hash it and create user with password
	// If password is empty/null, create user without password (will need reset link)
	var hashedPassword interface{} = nil
	if req.Password != nil && *req.Password != "" {
		hashedPassword, err = bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
	}

	_, err = db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, $3, $4, true)
	`, req.Email, hashedPassword, fullName, role)
	if err != nil {
		// Check if email already exists
		if strings.Contains(err.Error(), "duplicate key value") {
			http.Error(w, "Email already exists", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If no password was provided, generate reset token and send email
	if req.Password == nil || *req.Password == "" {
		// Get the newly created user ID
		var userID int
		err = db.QueryRow("SELECT id FROM users WHERE email = $1", req.Email).Scan(&userID)
		if err != nil {
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}

		// Generate reset token
		resetToken, err := generateResetToken()
		if err != nil {
			http.Error(w, "Failed to generate reset token", http.StatusInternalServerError)
			return
		}

		// Store reset token (expires in 24 hours)
		_, err = db.Exec(`
			INSERT INTO password_resets (user_id, token, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '24 hours')
		`, userID, resetToken)
		if err != nil {
			http.Error(w, "Failed to store reset token", http.StatusInternalServerError)
			return
		}

		// Get frontend URL from env or default
		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = "http://localhost:8080"
		}

		// Send welcome email with reset link
		subject := "Welcome to MoopicView - Set Your Password"
		resetLink := fmt.Sprintf("%s/reset-password?token=%s", frontendURL, resetToken)
		body := fmt.Sprintf(`
			<html>
			<body>
				<h1>Welcome to MoopicView</h1>
				<p>Hello %s,</p>
				<p>An account has been created for you on MoopicView.</p>
				<p>Please click the link below to set your password:</p>
				<p><a href="%s">Set Password</a></p>
				<p>This link will expire in 24 hours.</p>
				<p>Best regards,<br>MoopicView Admin</p>
			</body>
			</html>
		`, fullName, resetLink)

		if err := sendEmail(req.Email, subject, body); err != nil {
			log.Printf("Failed to send welcome email to %s: %v", req.Email, err)
			// We don't return an error here because the user was created successfully
		}
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

func adminUserDeleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Check if user exists
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Don't allow deleting the only admin
	var adminCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the user's role to check if they're an admin
	var userRole string
	err = db.QueryRow("SELECT role FROM users WHERE id = $1", id).Scan(&userRole)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if userRole == "admin" && adminCount <= 1 {
		http.Error(w, "Cannot delete the last admin user", http.StatusBadRequest)
		return
	}

	// Delete the user
	_, err = db.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "user deleted"})
}

func adminUserResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Get user details
	var email, name string
	err = db.QueryRow("SELECT email, name FROM users WHERE id = $1", id).Scan(&email, &name)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Generate reset token
	resetToken, err := generateResetToken()
	if err != nil {
		http.Error(w, "Failed to generate reset token", http.StatusInternalServerError)
		return
	}

	// Store reset token (expires in 24 hours)
	_, err = db.Exec(`
		INSERT INTO password_resets (user_id, token, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '24 hours')
	`, id, resetToken)
	if err != nil {
		http.Error(w, "Failed to store reset token", http.StatusInternalServerError)
		return
	}

	// Get frontend URL from env or default
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:8080"
	}

	// Send email with reset link
	subject := "MoopicView Password Reset"
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", frontendURL, resetToken)
	body := fmt.Sprintf(`
		<html>
		<body>
			<h1>MoopicView Password Reset</h1>
			<p>Hello %s,</p>
			<p>An admin has requested a password reset for your account.</p>
			<p>Please click the link below to set your new password:</p>
			<p><a href="%s">Reset Password</a></p>
			<p>This link will expire in 24 hours.</p>
			<p>Best regards,<br>MoopicView Admin</p>
		</body>
		</html>
	`, name, resetLink)

	if err := sendEmail(email, subject, body); err != nil {
		log.Printf("Failed to send password reset email to %s: %v", email, err)
		// We don't return an error here because the token was stored successfully
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset email sent"})
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

// Admin handlers for account requests
func adminAccountRequestsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, email, name, message, status, created_at 
		FROM account_requests 
		WHERE status = 'pending'
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	requests := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int
		var email, name, message, status string
		var createdAt time.Time
		rows.Scan(&id, &email, &name, &message, &status, &createdAt)
		requests = append(requests, map[string]interface{}{
			"id":         id,
			"email":      email,
			"name":       name,
			"message":    message,
			"status":     status,
			"created_at": createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

func adminAccountRequestReviewHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)

	var req struct {
		Status string `json:"status"` // "approved" or "rejected"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if req.Status != "approved" && req.Status != "rejected" {
		http.Error(w, "Status must be 'approved' or 'rejected'", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("postgres", getDBURL())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Get the account request details
	var email, name string
	err = db.QueryRow("SELECT email, name FROM account_requests WHERE id = $1", id).Scan(&email, &name)
	if err != nil {
		http.Error(w, "Account request not found", http.StatusNotFound)
		return
	}

	// If approved, create the user account and send reset link
	if req.Status == "approved" {
		// Create the user without password (they will set it via reset link)
		_, err = db.Exec(`
			INSERT INTO users (email, name, role, approved)
			VALUES ($1, $2, 'user', true)
		`, email, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get the newly created user ID
		var userID int
		err = db.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID)
		if err != nil {
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}

		// Generate reset token
		resetToken, err := generateResetToken()
		if err != nil {
			http.Error(w, "Failed to generate reset token", http.StatusInternalServerError)
			return
		}

		// Store reset token (expires in 24 hours)
		_, err = db.Exec(`
			INSERT INTO password_resets (user_id, token, expires_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '24 hours')
		`, userID, resetToken)
		if err != nil {
			http.Error(w, "Failed to store reset token", http.StatusInternalServerError)
			return
		}

		// Get frontend URL from env or default
		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = "http://localhost:8080"
		}

		// Send email with reset link
		subject := "MoopicView Account Approved - Set Your Password"
		resetLink := fmt.Sprintf("%s/reset-password?token=%s", frontendURL, resetToken)
		body := fmt.Sprintf(`
			<html>
			<body>
				<h1>MoopicView Account Approved</h1>
				<p>Hello %s,</p>
				<p>Your account request for MoopicView has been approved.</p>
				<p>Please click the link below to set your password:</p>
				<p><a href="%s">Set Password</a></p>
				<p>This link will expire in 24 hours.</p>
				<p>Best regards,<br>MoopicView Admin</p>
			</body>
			</html>
		`, name, resetLink)

		if err := sendEmail(email, subject, body); err != nil {
			log.Printf("Failed to send approval email to %s: %v", email, err)
			// We don't return an error here because the user was created successfully
		}
	}

	// Update the account request status
	_, err = db.Exec(`
		UPDATE account_requests 
		SET status = $1, reviewed_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, req.Status, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "request reviewed"})
}

func generateResetToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b), nil
}

func passwordResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Serve the reset password page
		spaHandler(w, r)
		return
	}

	if r.Method == "POST" {
		// Handle password reset
		var req struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		if req.Token == "" || req.Password == "" {
			http.Error(w, "Token and password are required", http.StatusBadRequest)
			return
		}

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

		// Verify token and check expiration
		var userID int
		err = db.QueryRow(`
			SELECT user_id FROM password_resets 
			WHERE token = $1 AND expires_at > CURRENT_TIMESTAMP
		`, req.Token).Scan(&userID)
		if err != nil {
			http.Error(w, "Invalid or expired reset token", http.StatusUnauthorized)
			return
		}

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}

		// Update user password
		_, err = db.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(hashedPassword), userID)
		if err != nil {
			http.Error(w, "Failed to update password", http.StatusInternalServerError)
			return
		}

		// Delete the used reset token
		_, err = db.Exec("DELETE FROM password_resets WHERE token = $1", req.Token)
		if err != nil {
			log.Printf("Failed to delete reset token: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "password updated"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
