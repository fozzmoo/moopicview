package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// Setup test database
func setupTestDB(t *testing.T) *sql.DB {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Drop and recreate users table (using the same name as production)
	_, err = db.Exec(`
		DROP TABLE IF EXISTS users CASCADE;
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			name TEXT,
			role TEXT DEFAULT 'user',
			approved BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test tables: %v", err)
	}

	// Drop and recreate password_resets table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS password_resets CASCADE;
		CREATE TABLE password_resets (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			token TEXT UNIQUE NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create password_resets table: %v", err)
	}

	// Drop and recreate photos table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS photos CASCADE;
		CREATE TABLE photos (
			id SERIAL PRIMARY KEY,
			filepath VARCHAR(500) UNIQUE NOT NULL,
			filename VARCHAR(255) NOT NULL,
			collection VARCHAR(20),
			folder_id INTEGER,
			photo_date DATE,
			date_precision VARCHAR(10),
			date_source VARCHAR(20),
			description TEXT,
			scan_date DATE,
			imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create photos table: %v", err)
	}

	// Drop and recreate comments table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS comments CASCADE;
		CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create comments table: %v", err)
	}

	// Drop and recreate tags table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS tags CASCADE;
		CREATE TABLE tags (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tags table: %v", err)
	}

	// Drop and recreate photo_tags table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS photo_tags CASCADE;
		CREATE TABLE photo_tags (
			photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
			tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
			pos_x FLOAT DEFAULT 50,
			pos_y FLOAT DEFAULT 50,
			PRIMARY KEY (photo_id, tag_id)
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create photo_tags table: %v", err)
	}

	// Insert test user with proper bcrypt hash
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, 'Test Admin', 'admin', true)
	`, "testadmin@example.com", hashedPassword)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	return db
}

// Cleanup test database
func cleanupTestDB(db *sql.DB, t *testing.T) {
	_, err := db.Exec("DROP TABLE IF EXISTS users CASCADE;")
	if err != nil {
		t.Logf("Failed to cleanup test tables: %v", err)
	}
	db.Close()
}

// Test login handler
func TestLoginHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Test valid login
	loginData := map[string]string{
		"email":    "testadmin@example.com",
		"password": "admin123",
	}
	body, _ := json.Marshal(loginData)
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	loginHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, ok := response["token"]; !ok {
		t.Error("Expected token in response")
	}

	// Verify token is valid
	tokenString := response["token"]
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		t.Errorf("Token is not valid: %v", err)
	}

	// Test invalid credentials
	loginData["password"] = "wrongpassword"
	body, _ = json.Marshal(loginData)
	req = httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	loginHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for invalid credentials, got %d", w.Code)
	}
}

// Test admin users handler
func TestAdminUsersHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Test getting users
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	w := httptest.NewRecorder()

	adminUsersHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}

	if users[0]["email"] != "testadmin@example.com" {
		t.Errorf("Expected email testadmin@example.com, got %v", users[0]["email"])
	}
}

// Test admin user approve handler
func TestAdminUserApproveHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert a pending user
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.DefaultCost)
	_, err := db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, 'Pending User', 'user', false)
	`, "pending@example.com", hashedPassword)
	if err != nil {
		t.Fatalf("Failed to insert pending user: %v", err)
	}

	// Get the user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", "pending@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}

	// Test approving user
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/users/%d/approve", userID), nil)
	vars := map[string]string{"id": fmt.Sprintf("%d", userID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminUserApproveHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user is approved
	var approved bool
	err = db.QueryRow("SELECT approved FROM users WHERE id = $1", userID).Scan(&approved)
	if err != nil {
		t.Fatalf("Failed to check user approval: %v", err)
	}

	if !approved {
		t.Error("User should be approved after calling adminUserApproveHandler")
	}
}

// Test health handler
func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	expected := "MoopicView is running"
	if w.Body.String() != expected {
		t.Errorf("Expected body '%s', got '%s'", expected, w.Body.String())
	}
}

// Test admin user change password handler
func TestAdminUserChangePasswordHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Get the admin user ID
	var userID int
	err := db.QueryRow("SELECT id FROM users WHERE email = $1", "testadmin@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}

	// Test changing password
	reqData := map[string]string{
		"newPassword": "newpassword123",
	}
	body, _ := json.Marshal(reqData)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/users/%d/change-password", userID), bytes.NewReader(body))
	vars := map[string]string{"id": fmt.Sprintf("%d", userID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminUserChangePasswordHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify password was updated by trying to login with new password
	// (This would require re-hashing and comparing, but for now we just check the response)
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "password updated" {
		t.Errorf("Expected status 'password updated', got '%s'", response["status"])
	}

	// Test changing password with too short password
	reqData["newPassword"] = "short"
	body, _ = json.Marshal(reqData)
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/admin/users/%d/change-password", userID), bytes.NewReader(body))
	req = mux.SetURLVars(req, vars)
	w = httptest.NewRecorder()

	adminUserChangePasswordHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for short password, got %d", w.Code)
	}
}

// Test admin create user handler
func TestAdminCreateUserHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Test creating a new user
	reqData := map[string]string{
		"first_name": "John",
		"last_name":  "Doe",
		"email":      "john.doe@example.com",
		"password":   "password123",
	}
	body, _ := json.Marshal(reqData)
	req := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body))
	w := httptest.NewRecorder()

	adminCreateUserHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "user created" {
		t.Errorf("Expected status 'user created', got '%s'", response["status"])
	}

	// Verify user was created in database
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE email = $1", "john.doe@example.com").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check user in database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 user with email john.doe@example.com, got %d", count)
	}

	// Test creating user with duplicate email
	reqData2 := map[string]string{
		"first_name": "Jane",
		"last_name":  "Doe",
		"email":      "john.doe@example.com",
		"password":   "password123",
	}
	body2, _ := json.Marshal(reqData2)
	req2 := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body2))
	w2 := httptest.NewRecorder()

	adminCreateUserHandler(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for duplicate email, got %d", w2.Code)
	}

	// Test creating user with missing fields
	reqData3 := map[string]string{
		"first_name": "Incomplete",
		// Missing last_name, email, password
	}
	body3, _ := json.Marshal(reqData3)
	req3 := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body3))
	w3 := httptest.NewRecorder()

	adminCreateUserHandler(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing fields, got %d", w3.Code)
	}

	// Test creating user with short password
	reqData4 := map[string]string{
		"first_name": "Short",
		"last_name":  "Password",
		"email":      "short@example.com",
		"password":   "123",
	}
	body4, _ := json.Marshal(reqData4)
	req4 := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body4))
	w4 := httptest.NewRecorder()

	adminCreateUserHandler(w4, req4)

	if w4.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for short password, got %d", w4.Code)
	}

	// Test creating user with admin role
	reqData5 := map[string]interface{}{
		"first_name": "Admin",
		"last_name":  "User",
		"email":      "adminuser@example.com",
		"password":   "password123",
		"is_admin":   true,
	}
	body5, _ := json.Marshal(reqData5)
	req5 := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body5))
	w5 := httptest.NewRecorder()

	adminCreateUserHandler(w5, req5)

	if w5.Code != http.StatusOK {
		t.Errorf("Expected status 200 for admin user creation, got %d: %s", w5.Code, w5.Body.String())
	}

	// Verify admin user was created with admin role
	var role string
	err = db.QueryRow("SELECT role FROM users WHERE email = $1", "adminuser@example.com").Scan(&role)
	if err != nil {
		t.Fatalf("Failed to check user role in database: %v", err)
	}
	if role != "admin" {
		t.Errorf("Expected role 'admin', got '%s'", role)
	}
}

// Test admin toggle admin handler
func TestAdminUserToggleAdminHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert a test user
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	_, err := db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, $3, 'user', true)
	`, "toggleuser@example.com", string(hashedPassword), "Toggle User")
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Get the user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", "toggleuser@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}

	// Test toggling from user to admin
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/users/%d/toggle-admin", userID), nil)
	vars := map[string]string{"id": fmt.Sprintf("%d", userID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminUserToggleAdminHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user is now admin
	var role string
	err = db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to check user role: %v", err)
	}
	if role != "admin" {
		t.Errorf("Expected role 'admin', got '%s'", role)
	}

	// Test toggling from admin back to user
	req2 := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/users/%d/toggle-admin", userID), nil)
	vars2 := map[string]string{"id": fmt.Sprintf("%d", userID)}
	req2 = mux.SetURLVars(req2, vars2)
	w2 := httptest.NewRecorder()

	adminUserToggleAdminHandler(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify user is now regular user
	err = db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to check user role: %v", err)
	}
	if role != "user" {
		t.Errorf("Expected role 'user', got '%s'", role)
	}
}

// Test admin photo date handler
func TestAdminPhotoDateHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL for tests
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		return "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert a test photo
	_, err := db.Exec(`
		INSERT INTO photos (filepath, filename, collection, photo_date, date_precision, date_source)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, "/test/photo.jpg", "test.jpg", "digital", "2020-01-15", "exact", "exif")
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Get the photo ID
	var photoID int
	err = db.QueryRow("SELECT id FROM photos WHERE filename = $1", "test.jpg").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	// Test updating photo date to month precision
	reqData := map[string]string{
		"photo_date":     "1989-06-01",
		"date_precision": "month",
	}
	body, _ := json.Marshal(reqData)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/photos/%d/date", photoID), bytes.NewReader(body))
	vars := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminPhotoDateHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify photo date was updated
	var photoDate string
	var datePrecision string
	err = db.QueryRow("SELECT photo_date::text, date_precision FROM photos WHERE id = $1", photoID).Scan(&photoDate, &datePrecision)
	if err != nil {
		t.Fatalf("Failed to check photo date: %v", err)
	}
	if photoDate != "1989-06-01" {
		t.Errorf("Expected photo_date '1989-06-01', got '%s'", photoDate)
	}
	if datePrecision != "month" {
		t.Errorf("Expected date_precision 'month', got '%s'", datePrecision)
	}

	// Test updating to exact date
	reqData2 := map[string]string{
		"photo_date":     "2020-06-25",
		"date_precision": "exact",
	}
	body2, _ := json.Marshal(reqData2)
	req2 := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/photos/%d/date", photoID), bytes.NewReader(body2))
	vars2 := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req2 = mux.SetURLVars(req2, vars2)
	w2 := httptest.NewRecorder()

	adminPhotoDateHandler(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify photo date was updated to exact
	err = db.QueryRow("SELECT photo_date::text, date_precision FROM photos WHERE id = $1", photoID).Scan(&photoDate, &datePrecision)
	if err != nil {
		t.Fatalf("Failed to check photo date: %v", err)
	}
	if photoDate != "2020-06-25" {
		t.Errorf("Expected photo_date '2020-06-25', got '%s'", photoDate)
	}
	if datePrecision != "exact" {
		t.Errorf("Expected date_precision 'exact', got '%s'", datePrecision)
	}

	// Test invalid precision
	reqData3 := map[string]string{
		"photo_date":     "2020-06-25",
		"date_precision": "invalid",
	}
	body3, _ := json.Marshal(reqData3)
	req3 := httptest.NewRequest("POST", fmt.Sprintf("/api/admin/photos/%d/date", photoID), bytes.NewReader(body3))
	vars3 := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req3 = mux.SetURLVars(req3, vars3)
	w3 := httptest.NewRecorder()

	adminPhotoDateHandler(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid precision, got %d", w3.Code)
	}
}

// Test photo content handler
func TestPhotoContentHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Set environment variable for database URL
	testDBURL := os.Getenv("TEST_DATABASE_URL")
	if testDBURL == "" {
		testDBURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	os.Setenv("DATABASE_URL", testDBURL)
	defer os.Unsetenv("DATABASE_URL")

	// Use a file path that exists in the container's mounted volume
	testImagePath := "/opt/mooview/digital/2025/20250704/P2430777.JPG"
	
	// Verify the file exists
	if _, err := os.Stat(testImagePath); os.IsNotExist(err) {
		t.Skip("Test file not found, skipping test")
	}

	// Insert a test photo with the existing file path
	_, err := db.Exec(`
		INSERT INTO photos (filepath, filename, collection, photo_date, date_precision, date_source, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (filepath) DO NOTHING
	`, testImagePath, "P2430777.JPG", "digital", "2025-07-04", "exact", "exif", "Test photo")
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Get the photo ID
	var photoID int
	err = db.QueryRow("SELECT id FROM photos WHERE filepath = $1", testImagePath).Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	t.Logf("Test photo ID: %d, path: %s", photoID, testImagePath)

	// Verify the photo exists in the database using the same connection method as the handler
	testDB, dbErr := sql.Open("postgres", testDBURL)
	if dbErr != nil {
		t.Fatalf("Failed to connect to test database: %v", dbErr)
	}
	defer testDB.Close()

	var verifyPath string
	err = testDB.QueryRow("SELECT filepath FROM photos WHERE id = $1", photoID).Scan(&verifyPath)
	if err != nil {
		t.Fatalf("Failed to verify photo in database: %v", err)
	}
	t.Logf("Verified photo path in database: %s", verifyPath)

	// Test the photo content handler
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/photos/%d/content", photoID), nil)
	vars := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	photoContentHandler(w, req)

	t.Logf("Response status: %d, body: %s", w.Code, w.Body.String())

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Errorf("Expected Content-Type 'image/jpeg', got '%s'", contentType)
	}

	// Verify content
	if len(w.Body.Bytes()) == 0 {
		t.Errorf("Expected photo content, got empty body")
	}

	// Test with non-existent photo ID
	req2 := httptest.NewRequest("GET", "/api/photos/99999/content", nil)
	vars2 := map[string]string{"id": "99999"}
	req2 = mux.SetURLVars(req2, vars2)
	w2 := httptest.NewRecorder()

	photoContentHandler(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent photo, got %d", w2.Code)
	}
}

func TestPhotoThumbnailHandler(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Set environment variables
	testDBURL := os.Getenv("TEST_DATABASE_URL")
	if testDBURL == "" {
		testDBURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
	}
	os.Setenv("DATABASE_URL", testDBURL)
	defer os.Unsetenv("DATABASE_URL")

	// Set cache directory for testing
	testCacheDir := "/opt/mooview/cache_test"
	os.Setenv("THUMBNAIL_CACHE_DIR", testCacheDir)
	defer os.Unsetenv("THUMBNAIL_CACHE_DIR")
	defer os.RemoveAll(testCacheDir)

	// Use a file path that exists in the container's mounted volume
	testImagePath := "/opt/mooview/digital/2025/20250704/P2430777.JPG"
	
	// Verify the file exists
	if _, err := os.Stat(testImagePath); os.IsNotExist(err) {
		t.Skip("Test file not found, skipping test")
	}

	// Insert a test photo with the existing file path
	_, err := db.Exec(`
		INSERT INTO photos (filepath, filename, collection, photo_date, date_precision, date_source, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (filepath) DO NOTHING
	`, testImagePath, "P2430777.JPG", "digital", "2025-07-04", "exact", "exif", "Test photo")
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Get the photo ID
	var photoID int
	err = db.QueryRow("SELECT id FROM photos WHERE filepath = $1", testImagePath).Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	t.Logf("Test photo ID: %d, path: %s", photoID, testImagePath)

	// Test the thumbnail handler
	req := httptest.NewRequest("GET", fmt.Sprintf("/thumbnails/%d", photoID), nil)
	vars := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	photoThumbnailHandler(w, req)

	t.Logf("Response status: %d, body length: %d", w.Code, len(w.Body.Bytes()))

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Errorf("Expected Content-Type 'image/jpeg', got '%s'", contentType)
	}

	// Verify content (thumbnail should be smaller than original)
	if len(w.Body.Bytes()) == 0 {
		t.Errorf("Expected thumbnail content, got empty body")
	}

	// Test with non-existent photo ID
	req2 := httptest.NewRequest("GET", "/thumbnails/99999", nil)
	vars2 := map[string]string{"id": "99999"}
	req2 = mux.SetURLVars(req2, vars2)
	w2 := httptest.NewRecorder()

	photoThumbnailHandler(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent photo, got %d", w2.Code)
	}

	// Test HEAD request
	req3 := httptest.NewRequest("HEAD", fmt.Sprintf("/thumbnails/%d", photoID), nil)
	vars3 := map[string]string{"id": fmt.Sprintf("%d", photoID)}
	req3 = mux.SetURLVars(req3, vars3)
	w3 := httptest.NewRecorder()

	photoThumbnailHandler(w3, req3)

	if w3.Code != http.StatusOK {
		t.Errorf("Expected status 200 for HEAD request, got %d", w3.Code)
	}

	// Verify content type for HEAD request
	contentType3 := w3.Header().Get("Content-Type")
	if contentType3 != "image/jpeg" {
		t.Errorf("Expected Content-Type 'image/jpeg' for HEAD request, got '%s'", contentType3)
	}

	// Verify HEAD request has no body
	if len(w3.Body.Bytes()) != 0 {
		t.Errorf("Expected empty body for HEAD request, got %d bytes", len(w3.Body.Bytes()))
	}
}

func TestAdminUserResetPasswordHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a test user
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	_, err := db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, $3, $4, $5)
	`, "testuser@example.com", string(hashedPassword), "Test User", "user", true)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Get the user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", "testuser@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get user ID: %v", err)
	}

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Create request to reset password
	req := httptest.NewRequest("POST", "/api/admin/users/"+fmt.Sprint(userID)+"/reset-password", nil)
	vars := map[string]string{"id": fmt.Sprint(userID)}
	req = mux.SetURLVars(req, vars)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	adminUserResetPasswordHandler(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify password reset token was created
	var tokenCount int
	err = db.QueryRow("SELECT COUNT(*) FROM password_resets WHERE user_id = $1", userID).Scan(&tokenCount)
	if err != nil {
		t.Fatalf("Failed to query password_resets table: %v", err)
	}
	if tokenCount != 1 {
		t.Errorf("Expected 1 password reset token, got %d", tokenCount)
	}

	// Test with non-existent user
	req2 := httptest.NewRequest("POST", "/api/admin/users/99999/reset-password", nil)
	vars2 := map[string]string{"id": "99999"}
	req2 = mux.SetURLVars(req2, vars2)
	req2.Header.Set("Authorization", "Bearer test-token")
	w2 := httptest.NewRecorder()

	adminUserResetPasswordHandler(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent user, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

// Test photoTagsHandler
func TestPhotoTagsHandler(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert test photo
	var photoID int
	err := db.QueryRow("INSERT INTO photos (filepath, filename, collection) VALUES ($1, $2, $3) RETURNING id", 
		"/test/photo.jpg", "photo.jpg", "test").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Insert test tags
	var tag1ID, tag2ID int
	err = db.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Person A").Scan(&tag1ID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}
	err = db.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Person B").Scan(&tag2ID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}

	// Associate tags with photo
	_, err = db.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tag1ID)
	if err != nil {
		t.Fatalf("Failed to associate tag with photo: %v", err)
	}
	_, err = db.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tag2ID)
	if err != nil {
		t.Fatalf("Failed to associate tag with photo: %v", err)
	}

	// Test getting tags for photo
	req := httptest.NewRequest("GET", "/api/photos/"+fmt.Sprint(photoID)+"/tags", nil)
	vars := map[string]string{"id": fmt.Sprint(photoID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	photoTagsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tags []map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &tags)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}

	// Test with non-existent photo
	req2 := httptest.NewRequest("GET", "/api/photos/99999/tags", nil)
	vars2 := map[string]string{"id": "99999"}
	req2 = mux.SetURLVars(req2, vars2)
	w2 := httptest.NewRecorder()

	photoTagsHandler(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent photo, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

// Test tagsHandler
func TestTagsHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert test tags
	_, err := testDB.Exec("INSERT INTO tags (name) VALUES ($1), ($2)", "Tag 1", "Tag 2")
	if err != nil {
		t.Fatalf("Failed to insert test tags: %v", err)
	}

	// Get the tag IDs for linking photos
	var tag1ID, tag2ID int
	err = testDB.QueryRow("SELECT id FROM tags WHERE name = $1", "Tag 1").Scan(&tag1ID)
	if err != nil {
		t.Fatalf("Failed to get tag1 ID: %v", err)
	}
	err = testDB.QueryRow("SELECT id FROM tags WHERE name = $1", "Tag 2").Scan(&tag2ID)
	if err != nil {
		t.Fatalf("Failed to get tag2 ID: %v", err)
	}

	// Insert a test photo and link it to Tag 1 only
	_, err = testDB.Exec(`
		INSERT INTO photos (filepath, filename, collection, folder_id) 
		VALUES ('/tmp/test/photo1.jpg', 'photo1.jpg', 'digital', 1)
		ON CONFLICT (filepath) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}
	var photoID int
	err = testDB.QueryRow("SELECT id FROM photos WHERE filepath = '/tmp/test/photo1.jpg'").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	_, err = testDB.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tag1ID)
	if err != nil {
		t.Fatalf("Failed to link photo to tag: %v", err)
	}

	// Test getting all tags
	req := httptest.NewRequest("GET", "/api/tags", nil)
	w := httptest.NewRecorder()

	tagsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tags []map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &tags)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Only Tag 1 has a photo, so it should be returned. Tag 2 has 0 photos and should be filtered out.
	if len(tags) != 1 {
		t.Errorf("Expected 1 tag (only tags with images), got %d", len(tags))
	}

	// Verify photo_count is present
	if len(tags) > 0 {
		if tags[0]["name"] != "Tag 1" {
			t.Errorf("Expected tag name 'Tag 1', got %v", tags[0]["name"])
		}
		if tags[0]["photo_count"].(float64) != 1 {
			t.Errorf("Expected photo_count 1, got %v", tags[0]["photo_count"])
		}
	}
}

// Test tagHandler
func TestTagHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert test tag
	var tagID int
	err := testDB.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Test Tag").Scan(&tagID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}

	// Insert a test photo and link it to the tag
	_, err = testDB.Exec(`
		INSERT INTO photos (filepath, filename, collection, folder_id) 
		VALUES ('/tmp/test/photo1.jpg', 'photo1.jpg', 'digital', 1)
		ON CONFLICT (filepath) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}
	var photoID int
	err = testDB.QueryRow("SELECT id FROM photos WHERE filepath = '/tmp/test/photo1.jpg'").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	_, err = testDB.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tagID)
	if err != nil {
		t.Fatalf("Failed to link photo to tag: %v", err)
	}

	// Test getting a single tag
	req := httptest.NewRequest("GET", "/api/tags/"+fmt.Sprint(tagID), nil)
	w := httptest.NewRecorder()

	// Use mux router to handle route variables
	r := mux.NewRouter()
	r.HandleFunc("/api/tags/{id}", tagHandler)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tag map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &tag)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if tag["name"] != "Test Tag" {
		t.Errorf("Expected tag name 'Test Tag', got %v", tag["name"])
	}

	// Verify photo_count is returned
	if tag["photo_count"].(float64) != 1 {
		t.Errorf("Expected photo_count 1, got %v", tag["photo_count"])
	}

	// Test getting non-existent tag
	req2 := httptest.NewRequest("GET", "/api/tags/99999", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent tag, got %d", w2.Code)
	}
}

// Test addPhotoTagHandler
func TestAddPhotoTagHandler(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert test photo
	var photoID int
	err := db.QueryRow("INSERT INTO photos (filepath, filename, collection) VALUES ($1, $2, $3) RETURNING id", 
		"/test/photo.jpg", "photo.jpg", "test").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Get test user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", "testadmin@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get test user: %v", err)
	}

	// Create JWT token for the user
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Test adding a new tag
	reqBody := `{"tagName": "New Person"}`
	req := httptest.NewRequest("POST", "/api/photos/"+fmt.Sprint(photoID)+"/tags", bytes.NewBufferString(reqBody))
	vars := map[string]string{"id": fmt.Sprint(photoID)}
	req = mux.SetURLVars(req, vars)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	// Set user ID in context
	ctx := context.WithValue(req.Context(), "user_id", userID)
	req = req.WithContext(ctx)

	addPhotoTagHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify tag was added to photo
	var tagCount int
	err = db.QueryRow("SELECT COUNT(*) FROM photo_tags WHERE photo_id = $1", photoID).Scan(&tagCount)
	if err != nil {
		t.Fatalf("Failed to count photo tags: %v", err)
	}
	if tagCount != 1 {
		t.Errorf("Expected 1 tag on photo, got %d", tagCount)
	}

	// Test adding duplicate tag
	req2 := httptest.NewRequest("POST", "/api/photos/"+fmt.Sprint(photoID)+"/tags", bytes.NewBufferString(reqBody))
	vars2 := map[string]string{"id": fmt.Sprint(photoID)}
	req2 = mux.SetURLVars(req2, vars2)
	req2.Header.Set("Authorization", "Bearer "+tokenString)
	w2 := httptest.NewRecorder()
	ctx2 := context.WithValue(req2.Context(), "user_id", userID)
	req2 = req2.WithContext(ctx2)

	addPhotoTagHandler(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for duplicate tag, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

// Test removePhotoTagHandler
func TestRemovePhotoTagHandler(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db, t)

	// Override getDBURL to use test database
	originalGetDBURL := getDBURL
	getDBURL = func() string {
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview_test?sslmode=disable"
		}
		return dbURL
	}
	defer func() { getDBURL = originalGetDBURL }()

	// Insert test photo
	var photoID int
	err := db.QueryRow("INSERT INTO photos (filepath, filename, collection) VALUES ($1, $2, $3) RETURNING id", 
		"/test/photo.jpg", "photo.jpg", "test").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}

	// Insert test tag
	var tagID int
	err = db.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Test Person").Scan(&tagID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}

	// Associate tag with photo
	_, err = db.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tagID)
	if err != nil {
		t.Fatalf("Failed to associate tag with photo: %v", err)
	}

	// Get test user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", "testadmin@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to get test user: %v", err)
	}

	// Create JWT token for the user
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Test removing tag from photo
	req := httptest.NewRequest("DELETE", "/api/photos/"+fmt.Sprint(photoID)+"/tags/"+fmt.Sprint(tagID), nil)
	vars := map[string]string{"id": fmt.Sprint(photoID), "tagId": fmt.Sprint(tagID)}
	req = mux.SetURLVars(req, vars)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	// Set user ID in context
	ctx := context.WithValue(req.Context(), "user_id", userID)
	req = req.WithContext(ctx)

	removePhotoTagHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify tag was removed from photo
	var tagCount int
	err = db.QueryRow("SELECT COUNT(*) FROM photo_tags WHERE photo_id = $1 AND tag_id = $2", photoID, tagID).Scan(&tagCount)
	if err != nil {
		t.Fatalf("Failed to count photo tags: %v", err)
	}
	if tagCount != 0 {
		t.Errorf("Expected 0 tags on photo after removal, got %d", tagCount)
	}
}

// Test adminListTagsHandler
func TestAdminListTagsHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Insert test tags
	_, err := testDB.Exec("INSERT INTO tags (name) VALUES ($1), ($2)", "Admin Tag 1", "Admin Tag 2")
	if err != nil {
		t.Fatalf("Failed to insert test tags: %v", err)
	}

	// Test getting all tags
	req := httptest.NewRequest("GET", "/api/admin/tags", nil)
	w := httptest.NewRecorder()

	adminListTagsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tags []map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &tags)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}

	// Verify photo_count is included
	if len(tags) > 0 {
		if tags[0]["photo_count"].(float64) != 0 {
			t.Errorf("Expected photo_count 0 for tag with no photos, got %v", tags[0]["photo_count"])
		}
	}
}

// Test adminCreateTagHandler
func TestAdminCreateTagHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Test creating a new tag
	reqBody := `{"name": "New Admin Tag"}`
	req := httptest.NewRequest("POST", "/api/admin/tags", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	adminCreateTagHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tag map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &tag)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if tag["name"] != "New Admin Tag" {
		t.Errorf("Expected tag name 'New Admin Tag', got %v", tag["name"])
	}

	// Test creating duplicate tag
	req2 := httptest.NewRequest("POST", "/api/admin/tags", strings.NewReader(reqBody))
	w2 := httptest.NewRecorder()

	adminCreateTagHandler(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for duplicate tag, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

// Test adminUpdateTagHandler
func TestAdminUpdateTagHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Insert a test tag
	var tagID int
	err := testDB.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Old Tag Name").Scan(&tagID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}

	// Test updating the tag
	reqBody := `{"name": "Updated Tag Name"}`
	req := httptest.NewRequest("PUT", "/api/admin/tags/"+fmt.Sprint(tagID), strings.NewReader(reqBody))
	vars := map[string]string{"id": fmt.Sprint(tagID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminUpdateTagHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var tag map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &tag)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if tag["name"] != "Updated Tag Name" {
		t.Errorf("Expected tag name 'Updated Tag Name', got %v", tag["name"])
	}
}

// Test adminDeleteTagHandler
func TestAdminDeleteTagHandler(t *testing.T) {
	testDB := setupTestDB(t)
	defer cleanupTestDB(testDB, t)

	// Set global db for handler access
	origDB := db
	db = testDB
	defer func() { db = origDB }()

	// Insert a test tag
	var tagID int
	err := testDB.QueryRow("INSERT INTO tags (name) VALUES ($1) RETURNING id", "Tag To Delete").Scan(&tagID)
	if err != nil {
		t.Fatalf("Failed to insert test tag: %v", err)
	}

	// Insert a photo and link it to the tag
	_, err = testDB.Exec("INSERT INTO photos (filepath, filename, collection) VALUES ($1, $2, $3)", 
		"/tmp/test/photo.jpg", "photo.jpg", "digital")
	if err != nil {
		t.Fatalf("Failed to insert test photo: %v", err)
	}
	var photoID int
	err = testDB.QueryRow("SELECT id FROM photos WHERE filepath = '/tmp/test/photo.jpg'").Scan(&photoID)
	if err != nil {
		t.Fatalf("Failed to get photo ID: %v", err)
	}

	_, err = testDB.Exec("INSERT INTO photo_tags (photo_id, tag_id) VALUES ($1, $2)", photoID, tagID)
	if err != nil {
		t.Fatalf("Failed to link photo to tag: %v", err)
	}

	// Test deleting the tag
	req := httptest.NewRequest("DELETE", "/api/admin/tags/"+fmt.Sprint(tagID), nil)
	vars := map[string]string{"id": fmt.Sprint(tagID)}
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()

	adminDeleteTagHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify tag was deleted
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM tags WHERE id = $1)", tagID).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check if tag exists: %v", err)
	}
	if exists {
		t.Errorf("Tag should have been deleted but still exists")
	}

	// Verify photo tag association was also deleted
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM photo_tags WHERE tag_id = $1)", tagID).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check photo tag association: %v", err)
	}
	if exists {
		t.Errorf("Photo tag association should have been deleted but still exists")
	}
}
