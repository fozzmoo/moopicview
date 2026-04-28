package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

	// Drop and recreate photos table
	_, err = db.Exec(`
		DROP TABLE IF EXISTS photos CASCADE;
		CREATE TABLE photos (
			id SERIAL PRIMARY KEY,
			filepath VARCHAR(500) UNIQUE NOT NULL,
			filename VARCHAR(255) NOT NULL,
			collection VARCHAR(20),
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