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
	"sync"
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

var db *sql.DB

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
// extractToken gets the JWT from the Authorization header or the token cookie
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := r.Cookie("token"); err == nil {
		return cookie.Value
	}
	return ""
}

// spaAuthMiddleware checks authentication for SPA page loads.
// Redirects to /login if not authenticated.
func spaAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := extractToken(r)
		if tokenString == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := extractToken(r)
		if tokenString == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

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

	// Initialize shared database connection pool
	var err error
	db, err = sql.Open("postgres", getDBURL())
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	port := os.Getenv("LISTEN_ADDR")
	if port == "" {
		port = ":8080"
	}

	r := mux.NewRouter()

	// Public API routes (no auth required)
	publicAPI := r.PathPrefix("/api").Subrouter()
	publicAPI.HandleFunc("/auth/login", loginHandler).Methods("POST")
	publicAPI.HandleFunc("/auth/logout", logoutHandler).Methods("POST")
	publicAPI.HandleFunc("/auth/google", googleAuthHandler).Methods("GET")
	publicAPI.HandleFunc("/auth/google/callback", googleAuthCallbackHandler).Methods("GET")
	publicAPI.HandleFunc("/auth/request-access", requestAccessHandler).Methods("POST")
	publicAPI.HandleFunc("/auth/change-password", changePasswordHandler).Methods("POST")
	publicAPI.HandleFunc("/auth/reset-password", passwordResetHandler).Methods("POST")
	publicAPI.HandleFunc("/health", healthHandler).Methods("GET")

	// All other API routes require authentication
	authAPI := r.PathPrefix("/api").Subrouter()
	authAPI.Use(authMiddleware)
	authAPI.HandleFunc("/photos", photosHandler).Methods("GET")
	authAPI.HandleFunc("/photos/{id}", photoHandler).Methods("GET")
	authAPI.HandleFunc("/photos/{id}/content", photoContentHandler).Methods("GET")
	authAPI.HandleFunc("/photos/{id}/comments", photoCommentsHandler).Methods("GET")
	authAPI.HandleFunc("/photos/{id}/comments", addPhotoCommentHandler).Methods("POST")
	authAPI.HandleFunc("/photos/{id}/tags", photoTagsHandler).Methods("GET")
	authAPI.HandleFunc("/photos/{id}/tags", addPhotoTagHandler).Methods("POST")
	authAPI.HandleFunc("/photos/{id}/tags/{tagId}", removePhotoTagHandler).Methods("DELETE")
	authAPI.HandleFunc("/tags", tagsHandler).Methods("GET")
	authAPI.HandleFunc("/tags/{id}/photos", photosByTagHandler).Methods("GET")
	authAPI.HandleFunc("/collections", collectionsHandler).Methods("GET")
	authAPI.HandleFunc("/collections/{id}", collectionHandler).Methods("GET")
	authAPI.HandleFunc("/folders", foldersHandler).Methods("GET")

	// Thumbnail route (outside /api prefix, but still requires auth)
	r.Handle("/thumbnails/{id}", authMiddleware(http.HandlerFunc(photoThumbnailHandler))).Methods("GET", "HEAD")

	// Admin routes (require admin role)
	adminRouter := r.PathPrefix("/api/admin").Subrouter()
	adminRouter.Use(authMiddleware)
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
	adminRouter.HandleFunc("/scan", scanHandler).Methods("POST")

	// Static assets (public) - must be before spaAuth catch-all
	r.PathPrefix("/assets/").HandlerFunc(spaHandler).Methods("GET")
	r.PathPrefix("/favicon").HandlerFunc(spaHandler).Methods("GET")
	r.PathPrefix("/index.html").HandlerFunc(spaHandler).Methods("GET")

	// Public SPA routes (no auth required)
	r.HandleFunc("/login", spaHandler).Methods("GET")
	r.HandleFunc("/reset-password", spaHandler).Methods("GET")

	// Protected SPA routes (require authentication)
	spaAuth := r.NewRoute().Subrouter()
	spaAuth.Use(spaAuthMiddleware)
	spaAuth.HandleFunc("/", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/collections", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/collections/{id}", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/photo/{id}", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/tags", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/tags/{id}", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/account", spaHandler).Methods("GET")
	spaAuth.HandleFunc("/admin", spaHandler).Methods("GET")
	spaAuth.PathPrefix("/").HandlerFunc(spaHandler).Methods("GET")

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
	var err error
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}


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

	// Set JWT as httpOnly cookie for SPA page loads
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
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
	var err error
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
	var err error
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
	// Determine frontend dist path
	frontendDist := os.Getenv("FRONTEND_DIST")
	if frontendDist == "" {
		// Default to relative path for local development
		frontendDist = "../../frontend/dist"
	}

	// Check if the path is for a static asset
	path := r.URL.Path
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/favicon") || strings.HasPrefix(path, "/index.html") {
		// Serve the static file
		filePath := filepath.Join(frontendDist, path)
		http.ServeFile(w, r, filePath)
		return
	}

	// For all other routes, serve the index.html (SPA routing)
	http.ServeFile(w, r, filepath.Join(frontendDist, "index.html"))
}

func collectionsHandler(w http.ResponseWriter, r *http.Request) {
	rootsStr := os.Getenv("PHOTO_ROOTS")
	if rootsStr == "" {
		rootsStr = "digital:/unas/images/digital_photos/2017/20170625-FortBuenaVentura,scanned:/unas/images/scanned_photos/scan-date/2024/20240404"
	}
	rootEntries := strings.Split(rootsStr, ",")


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
		err := db.QueryRow(
			"SELECT id, name FROM folders WHERE path = $1",
			path,
		).Scan(&folderID, &name)
		if err != nil {
			log.Printf("Folder not found: %s", path)
			continue
		}

		// Count photos in this folder and its subfolders using a JOIN
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM photos p
			JOIN folders f ON p.folder_id = f.id
			WHERE f.path LIKE $1 OR f.path = $2
		`, path+"%", path).Scan(&count)
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
	var err error
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
	var err error
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

type pendingPhoto struct {
	filepath     string
	filename     string
	collection   string
	folderID     int
	photoDate    sql.NullString
	datePrecision string
	dateSource   string
}

func nullStringToInterface(n sql.NullString) interface{} {
	if n.Valid {
		return n.String
	}
	return nil
}

func flushPhotoBatch(tx *sql.Tx, batch []pendingPhoto) error {
	if len(batch) == 0 {
		return nil
	}
	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*8)
	for i, p := range batch {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, CURRENT_DATE, $%d, $%d, $%d, $%d)", i*8+1, i*8+2, i*8+3, i*8+4, i*8+5, i*8+6, i*8+7, i*8+8))
		valueArgs = append(valueArgs, p.filepath, p.filename, p.collection, p.folderID, nullStringToInterface(p.photoDate), p.datePrecision, p.dateSource, "Scanned photo")
	}
	query := fmt.Sprintf(`
		INSERT INTO photos (filepath, filename, collection, folder_id, scan_date, photo_date, date_precision, date_source, description)
		VALUES %s
		ON CONFLICT (filepath) DO UPDATE SET
			filename = EXCLUDED.filename,
			folder_id = EXCLUDED.folder_id,
			scan_date = CURRENT_DATE
	`, strings.Join(valueStrings, ","))
	_, err := tx.Exec(query, valueArgs...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func scanRoot(entry string) {
	parts := strings.SplitN(entry, ":", 2)
	photoType := "digital"
	path := ""
	if len(parts) == 2 {
		photoType = strings.TrimSpace(parts[0])
		path = strings.TrimSpace(parts[1])
	} else {
		path = strings.TrimSpace(parts[0])
	}

	// Load existing files from DB: filepath -> scan_date
	existingFiles := make(map[string]time.Time)
	rows, err := db.Query("SELECT filepath, scan_date FROM photos WHERE filepath LIKE $1 ESCAPE '/'", path+"%")
	if err != nil {
		log.Println("Load existing files error for", path, ":", err)
	} else {
		for rows.Next() {
			var filepath string
			var scanDate sql.NullTime
			rows.Scan(&filepath, &scanDate)
			if scanDate.Valid {
				existingFiles[filepath] = scanDate.Time
			}
		}
		rows.Close()
	}

	folderIDCache := make(map[string]int)
	folderExistsCache := make(map[string]bool)

	// Delete missing files
	for filepath := range existingFiles {
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			db.Exec("DELETE FROM photos WHERE filepath = $1", filepath)
			log.Println("Deleted:", filepath)
			delete(existingFiles, filepath)
		}
	}

	const batchSize = 500
	var batch []pendingPhoto

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := flushPhotoBatch(tx, batch); err != nil {
			tx.Rollback()
			return err
		}
		for _, p := range batch {
			log.Printf("Added/Updated: %s (type=%s, date=%v, precision=%s, source=%s)", p.filename, p.collection, p.photoDate.String, p.datePrecision, p.dateSource)
		}
		batch = batch[:0]
		return nil
	}

	// Add/update files
	filepath.WalkDir(path, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		nameLower := strings.ToLower(name)
		if strings.HasSuffix(nameLower, ".jpg") || strings.HasSuffix(nameLower, ".jpeg") || strings.HasSuffix(nameLower, ".png") {

			// Skip files already in DB
			if _, ok := existingFiles[fullPath]; ok {
				return nil
			}

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
			
			// Check folder ID cache first
			folderID, cached := folderIDCache[dirPath]
			if !cached {
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
				folderIDCache[dirPath] = folderID
				folderExistsCache[dirPath] = true
				
				// Insert all parent directories recursively (for folder navigation)
				currentPath := filepath.Dir(dirPath)
				for {
					if currentPath == "/" || currentPath == "." || currentPath == path {
						break
					}
					if folderExistsCache[currentPath] {
						break
					}
					parentPath := filepath.Dir(currentPath)
					
					_, err = db.Exec(`
						INSERT INTO folders (path, name, parent_path, collection_type)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (path) DO UPDATE SET name = EXCLUDED.name
					`, currentPath, filepath.Base(currentPath), parentPath, photoType)
					if err != nil {
						log.Printf("Error inserting parent folder %s: %v", currentPath, err)
					}
					folderExistsCache[currentPath] = true
					
					currentPath = parentPath
				}
			}

			batch = append(batch, pendingPhoto{
				filepath:      fullPath,
				filename:      name,
				collection:    photoType,
				folderID:      folderID,
				photoDate:     photoDate,
				datePrecision: datePrecision,
				dateSource:    dateSource,
			})

			if len(batch) >= batchSize {
				if err := flushBatch(); err != nil {
					log.Printf("Error flushing batch: %v", err)
				}
			}
		}
		return nil
	})

	// Flush remaining photos
	if err := flushBatch(); err != nil {
		log.Printf("Error flushing final batch: %v", err)
	}
	log.Printf("Scan complete for %s (%s)", path, photoType)
}

func scanPhotos() {

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

	var wg sync.WaitGroup
	for _, entry := range rootPaths {
		wg.Add(1)
		go func(e string) {
			defer wg.Done()
			scanRoot(e)
		}(entry)
	}
	wg.Wait()
	log.Println("Scan complete.")
}

func extractExifDate(filePath string) (time.Time, string, bool) {
	var err error
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
	var err error

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

	var filepathStr string
	err := db.QueryRow("SELECT filepath FROM photos WHERE id = $1", id).Scan(&filepathStr)
	if err != nil {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	// Determine cache directory
	cacheDir := os.Getenv("THUMBNAIL_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/mooview_cache"
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

			// Try to read EXIF orientation (only for larger files)
			var orientationVal int
			file, err := os.Open(filepathStr)
			if err == nil {
				if info, err := file.Stat(); err == nil && info.Size() > 100000 {
					exifData, err := exif.Decode(file)
					if err == nil {
						orientation, err := exifData.Get(exif.Orientation)
						if err == nil {
							orientationVal, _ = orientation.Int(0)
						}
					}
				}
				file.Close()
			}

			// Apply orientation if needed
			switch orientationVal {
			case 3: img = imaging.Rotate180(img)
			case 6: img = imaging.Rotate270(img)
			case 8: img = imaging.Rotate90(img)
			}

			// Resize using faster Box filter
			thumbnail := imaging.Resize(img, 300, 0, imaging.Box)

			// Save thumbnail
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

func getFileInfo(filepathStr string) string {
	ext := strings.ToLower(filepath.Ext(filepathStr))
	var fileType string
	switch ext {
	case ".jpg", ".jpeg":
		fileType = "JPEG"
	case ".png":
		fileType = "PNG"
	case ".gif":
		fileType = "GIF"
	case ".tiff", ".tif":
		fileType = "TIFF"
	case ".webp":
		fileType = "WebP"
	default:
		fileType = strings.TrimPrefix(ext, ".")
		if fileType == "" {
			fileType = "Unknown"
		}
	}

	fi, err := os.Stat(filepathStr)
	if err != nil {
		return fileType
	}
	size := fi.Size()
	var sizeStr string
	switch {
	case size >= 1<<20:
		sizeStr = fmt.Sprintf("%.1fMiB", float64(size)/(1<<20))
	case size >= 1<<10:
		sizeStr = fmt.Sprintf("%.1fKiB", float64(size)/(1<<10))
	default:
		sizeStr = fmt.Sprintf("%dB", size)
	}

	// Read image header for dimensions and colorspace
	colorInfo := ""
	f, err := os.Open(filepathStr)
	if err == nil {
		defer f.Close()
		header := make([]byte, 32)
		n, _ := f.Read(header)
		if n >= 2 && header[0] == 0xFF && header[1] == 0xD8 {
			// JPEG: read SOF marker for component info
			f.Seek(2, io.SeekStart)
			buf := make([]byte, 16)
			for {
				if _, err := f.Read(buf[:2]); err != nil {
					break
				}
				if buf[0] != 0xFF {
					break
				}
				marker := buf[1]
				if marker == 0x00 || marker == 0xFF {
					continue
				}
				if marker == 0xD9 || marker == 0xDA {
					break
				}
				// SOF markers: 0xC0-0xCF (except 0xC4 DHT and 0xC8 JPEG extension)
				if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 {
					// SOF segment: 2-byte length, 1-byte precision, 2-byte height, 2-byte width, 1-byte numComponents
					if _, err := io.ReadFull(f, buf[:8]); err != nil {
						break
					}
					precision := int(buf[2])
					_ = int(buf[3])<<8 | int(buf[4]) // height
					_ = int(buf[5])<<8 | int(buf[6]) // width
					numComponents := int(buf[7])
					switch {
					case numComponents == 1:
						colorInfo = fmt.Sprintf("%d-bit grayscale", precision)
					case numComponents == 3:
						colorInfo = fmt.Sprintf("%d-bit color", precision)
					case numComponents == 4:
						colorInfo = fmt.Sprintf("%d-bit CMYK", precision)
					default:
						colorInfo = fmt.Sprintf("%d-bit %dch", precision, numComponents)
					}
					break
				}
				// Skip this marker's payload
				if _, err := io.ReadFull(f, buf[:2]); err != nil {
					break
				}
				length := int(buf[0])<<8 | int(buf[1])
				if length < 2 {
					break
				}
				io.CopyN(io.Discard, f, int64(length-2))
			}
		} else if n >= 16 && string(header[4:8]) == "IHDR" {
			// PNG: IHDR chunk data starts at byte 8
			// Width: bytes 8-11, Height: bytes 12-15, Bit depth: byte 16, Color type: byte 17
			if n >= 18 {
				bitDepth := int(header[16])
				colorType := int(header[17])
				switch colorType {
				case 0:
					colorInfo = fmt.Sprintf("%d-bit grayscale", bitDepth)
				case 2:
					colorInfo = fmt.Sprintf("%d-bit color", bitDepth)
				case 3:
					colorInfo = fmt.Sprintf("%d-bit indexed", bitDepth)
				case 4:
					colorInfo = fmt.Sprintf("%d-bit grayscale+alpha", bitDepth)
				case 6:
					colorInfo = fmt.Sprintf("%d-bit color+alpha", bitDepth)
				}
			}
		}
	}

	dimensions := ""
	if width, height, ok := getPhotoDimensions(filepathStr); ok {
		dimensions = fmt.Sprintf("%dx%d", width, height)
	}

	parts := []string{fileType}
	if colorInfo != "" {
		parts = append(parts, colorInfo)
	}
	parts = append(parts, sizeStr)
	if dimensions != "" {
		parts = append(parts, dimensions)
	}
	return strings.Join(parts, " - ")
}

func getPhotoDimensions(filepathStr string) (int, int, bool) {
	f, err := os.Open(filepathStr)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	header := make([]byte, 32)
	n, _ := f.Read(header)
	if n < 2 {
		return 0, 0, false
	}
	if header[0] == 0xFF && header[1] == 0xD8 {
		// JPEG
		f.Seek(2, io.SeekStart) // seek back after SOI
		buf := make([]byte, 16)
		for {
			if _, err := f.Read(buf[:2]); err != nil {
				break
			}
			if buf[0] != 0xFF {
				break
			}
			marker := buf[1]
			if marker == 0xD9 || marker == 0xDA {
				break
			}
			if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 {
				if _, err := io.ReadFull(f, buf[:8]); err != nil {
					break
				}
				height := int(buf[3])<<8 | int(buf[4])
				width := int(buf[5])<<8 | int(buf[6])
				return width, height, true
			}
			if _, err := io.ReadFull(f, buf[:2]); err != nil {
				break
			}
			length := int(buf[0])<<8 | int(buf[1])
			if length < 2 {
				break
			}
			io.CopyN(io.Discard, f, int64(length-2))
		}
	} else if n >= 16 && string(header[4:8]) == "IHDR" {
		// PNG: Width at bytes 8-11, Height at bytes 12-15
		width := int(header[8])<<24 | int(header[9])<<16 | int(header[10])<<8 | int(header[11])
		height := int(header[12])<<24 | int(header[13])<<16 | int(header[14])<<8 | int(header[15])
		return width, height, true
	}
	return 0, 0, false
}

func photoHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


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

	// Build file info string
	fileInfo := getFileInfo(photo.Filepath)

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

	// Get tags for this photo
	tagRows, err := db.Query(`
		SELECT t.id, t.name, pt.pos_x, pt.pos_y
		FROM tags t
		JOIN photo_tags pt ON t.id = pt.tag_id
		WHERE pt.photo_id = $1
		ORDER BY t.name ASC
	`, id)
	if err != nil {
		http.Error(w, "Failed to fetch tags", http.StatusInternalServerError)
		return
	}
	defer tagRows.Close()

	tags := []map[string]interface{}{}
	for tagRows.Next() {
		var tag struct {
			ID    int     `json:"id"`
			Name  string  `json:"name"`
			PosX  float64 `json:"posX"`
			PosY  float64 `json:"posY"`
		}
		err := tagRows.Scan(&tag.ID, &tag.Name, &tag.PosX, &tag.PosY)
		if err != nil {
			continue
		}
		tags = append(tags, map[string]interface{}{
			"id":    tag.ID,
			"name":  tag.Name,
			"posX":  tag.PosX,
			"posY":  tag.PosY,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"photo":       photo,
		"file_info":   fileInfo,
		"breadcrumbs": breadcrumbs,
		"comments":    comments,
		"tags":        tags,
	})
}

// photoCommentsHandler returns all comments for a photo
func photoCommentsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


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

// photoTagsHandler returns all tags for a photo
func photoTagsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


	// Check if photo exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM photos WHERE id = $1)", id).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT t.id, t.name, pt.pos_x, pt.pos_y
		FROM tags t
		JOIN photo_tags pt ON t.id = pt.tag_id
		WHERE pt.photo_id = $1
		ORDER BY t.name ASC
	`, id)
	if err != nil {
		http.Error(w, "Failed to fetch tags", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tags := []map[string]interface{}{}
	for rows.Next() {
		var tag struct {
			ID    int     `json:"id"`
			Name  string  `json:"name"`
			PosX  float64 `json:"posX"`
			PosY  float64 `json:"posY"`
		}
		err := rows.Scan(&tag.ID, &tag.Name, &tag.PosX, &tag.PosY)
		if err != nil {
			continue
		}
		tags = append(tags, map[string]interface{}{
			"id":    tag.ID,
			"name":  tag.Name,
			"posX":  tag.PosX,
			"posY":  tag.PosY,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// tagsHandler returns all available tags (for autocomplete)
func tagsHandler(w http.ResponseWriter, r *http.Request) {

	rows, err := db.Query("SELECT id, name FROM tags ORDER BY name ASC")
	if err != nil {
		http.Error(w, "Failed to fetch tags", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tags := []map[string]interface{}{}
	for rows.Next() {
		var tag struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		err := rows.Scan(&tag.ID, &tag.Name)
		if err != nil {
			continue
		}
		tags = append(tags, map[string]interface{}{
			"id":   tag.ID,
			"name": tag.Name,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// photosByTagHandler returns all photos for a specific tag
func photosByTagHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tagIdStr := vars["id"]
	tagID, _ := strconv.Atoi(tagIdStr)


	// Check if tag exists
	var tagName string
	err := db.QueryRow("SELECT name FROM tags WHERE id = $1", tagID).Scan(&tagName)
	if err != nil {
		http.Error(w, "Tag not found", http.StatusNotFound)
		return
	}

	// Get photos with this tag
	rows, err := db.Query(`
		SELECT p.id, p.filepath, p.filename, p.collection, p.folder_id, 
		       p.photo_date, p.date_precision, p.date_source, p.description,
		       pt.pos_x, pt.pos_y
		FROM photos p
		JOIN photo_tags pt ON p.id = pt.photo_id
		WHERE pt.tag_id = $1
		ORDER BY p.photo_date DESC, p.id DESC
	`, tagID)
	if err != nil {
		http.Error(w, "Failed to fetch photos", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	photos := []map[string]interface{}{}
	for rows.Next() {
		var photo struct {
			ID            int       `json:"id"`
			Filepath      string    `json:"filepath"`
			Filename      string    `json:"filename"`
			Collection    string    `json:"collection"`
			FolderID      *int      `json:"folder_id"`
			PhotoDate     *string   `json:"photo_date"`
			DatePrecision string    `json:"date_precision"`
			DateSource    string    `json:"date_source"`
			Description   string    `json:"description"`
			PosX          float64   `json:"posX"`
			PosY          float64   `json:"posY"`
		}
		err := rows.Scan(&photo.ID, &photo.Filepath, &photo.Filename, &photo.Collection, 
			&photo.FolderID, &photo.PhotoDate, &photo.DatePrecision, &photo.DateSource,
			&photo.Description, &photo.PosX, &photo.PosY)
		if err != nil {
			continue
		}
		
		photos = append(photos, map[string]interface{}{
			"id":            photo.ID,
			"filepath":      photo.Filepath,
			"filename":      photo.Filename,
			"collection":    photo.Collection,
			"folder_id":     photo.FolderID,
			"photo_date":    photo.PhotoDate,
			"date_precision": photo.DatePrecision,
			"date_source":   photo.DateSource,
			"description":   photo.Description,
			"content_url":   fmt.Sprintf("/api/photos/%d/content", photo.ID),
			"tag_position": map[string]interface{}{
				"x": photo.PosX,
				"y": photo.PosY,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tag_id":   tagID,
		"tag_name": tagName,
		"photos":   photos,
		"count":    len(photos),
	})
}

// addPhotoTagHandler adds a tag to a photo
func addPhotoTagHandler(w http.ResponseWriter, r *http.Request) {
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
		TagName string  `json:"tagName"`
		PosX    *float64 `json:"posX"`
		PosY    *float64 `json:"posY"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sanitize tag name
	tagName := strings.TrimSpace(req.TagName)
	if tagName == "" {
		http.Error(w, "Tag name is required", http.StatusBadRequest)
		return
	}

	// Default position to center (50, 50) if not provided
	posX := 50.0
	posY := 50.0
	if req.PosX != nil {
		posX = *req.PosX
	}
	if req.PosY != nil {
		posY = *req.PosY
	}

	// Validate position is within bounds (0-100)
	if posX < 0 || posX > 100 || posY < 0 || posY > 100 {
		http.Error(w, "Position must be between 0 and 100", http.StatusBadRequest)
		return
	}


	// Upsert tag
	var tagID int
	err := db.QueryRow("INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id", tagName).Scan(&tagID)
	if err != nil {
		http.Error(w, "Failed to create tag", http.StatusInternalServerError)
		return
	}

	// Add tag to photo (ON CONFLICT handles duplicate detection)
	result, err := db.Exec("INSERT INTO photo_tags (photo_id, tag_id, pos_x, pos_y) VALUES ($1, $2, $3, $4) ON CONFLICT (photo_id, tag_id) DO NOTHING", photoID, tagID, posX, posY)
	if err != nil {
		http.Error(w, "Failed to add tag to photo", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Tag already exists for this photo", http.StatusBadRequest)
		return
	}

	// Log the activity
	log.Printf("User %d added tag '%s' to photo %d at position (%.1f, %.1f)", userID, tagName, photoID, posX, posY)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    tagID,
		"name":  tagName,
		"posX":  posX,
		"posY":  posY,
	})
}

// removePhotoTagHandler removes a tag from a photo
func removePhotoTagHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	photoID, _ := strconv.Atoi(idStr)
	tagIdStr := vars["tagId"]
	tagID, _ := strconv.Atoi(tagIdStr)

	// Get user from context (requires authentication)
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}


	// Check if association exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM photo_tags WHERE photo_id = $1 AND tag_id = $2)", photoID, tagID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Tag not found for this photo", http.StatusNotFound)
		return
	}

	// Remove tag from photo
	_, err = db.Exec("DELETE FROM photo_tags WHERE photo_id = $1 AND tag_id = $2", photoID, tagID)
	if err != nil {
		http.Error(w, "Failed to remove tag from photo", http.StatusInternalServerError)
		return
	}

	// Log the activity
	log.Printf("User %d removed tag %d from photo %d", userID, tagID, photoID)

	w.WriteHeader(http.StatusNoContent)
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	go scanPhotos()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scan started"})
}

// foldersHandler returns all folders for a given collection type or parent folder
func foldersHandler(w http.ResponseWriter, r *http.Request) {
	collectionType := r.URL.Query().Get("type")
	parentID := r.URL.Query().Get("parent")


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
		SELECT p.id, p.filepath, p.filename, p.collection, p.photo_date::text, p.date_precision, 
		       COUNT(pt.tag_id) as tag_count
		FROM photos p
		LEFT JOIN photo_tags pt ON p.id = pt.photo_id
		WHERE p.folder_id = $1
		GROUP BY p.id
		ORDER BY p.filename
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
		var tagCount int
		photoRows.Scan(&photoID, &filepath, &filename, &collection, &photoDate, &datePrecision, &tagCount)
		photos = append(photos, map[string]interface{}{
			"id":             photoID,
			"filename":       filename,
			"collection":     collection,
			"photo_date":     photoDate,
			"date_precision": datePrecision,
			"tag_count":      tagCount,
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
	var err error

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
	var err error
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
	var err error
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


	_, err = db.Exec("UPDATE users SET approved = TRUE WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

func adminUserChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	var err error
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
	var err error

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
	var err error
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
	var err error
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


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
	var err error
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


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
	var err error
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, _ := strconv.Atoi(idStr)


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
	var err error
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
	var err error
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
	var err error

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
	var err error
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
	var err error
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
