package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dbURL := os.Getenv("CLI_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview?sslmode=disable"
	}

	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@fozzilinymoo.org"
	}
	password := "admin123" // Change immediately after first login

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create all tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			name TEXT,
			google_id TEXT UNIQUE,
			role TEXT DEFAULT 'user',
			approved BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS account_requests (
			id SERIAL PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			name TEXT,
			message TEXT,
			status TEXT DEFAULT 'pending',
			reviewed_by INTEGER REFERENCES users(id),
			reviewed_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS photos (
			id SERIAL PRIMARY KEY,
			filepath TEXT UNIQUE NOT NULL,
			filename TEXT NOT NULL,
			collection TEXT NOT NULL,
			scan_date DATE,
			photo_date DATE,
			date_precision VARCHAR(10) DEFAULT 'unknown',
			date_source VARCHAR(20) DEFAULT 'unknown',
			description TEXT,
			original_date TIMESTAMP,
			width INTEGER,
			height INTEGER,
			imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)

	`)
	if err != nil {
		log.Fatal(err)
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	// Upsert admin
	_, err = db.Exec(`
		INSERT INTO users (email, password_hash, name, role, approved)
		VALUES ($1, $2, 'Admin', 'admin', true)
		ON CONFLICT (email) DO UPDATE SET 
			password_hash = EXCLUDED.password_hash,
			role = 'admin',
			approved = true
	`, email, hashed)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Admin account created: %s / %s\n", email, password)
	fmt.Println("Change password immediately after first login.")
}
