package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("CLI_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Drop existing tables if they exist (blow away schema)
	fmt.Println("Dropping existing tables...")
	_, _ = db.Exec(`DROP TABLE IF EXISTS activity_logs CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS proposed_edits CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS comments CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS photo_tags CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS tags CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS account_requests CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS users CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS photos CASCADE`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS folders CASCADE`)

	// Create folders table
	_, err = db.Exec(`
		CREATE TABLE folders (
			id SERIAL PRIMARY KEY,
			path VARCHAR(500) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			parent_path VARCHAR(500),
			collection_type VARCHAR(20),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create photos table
	_, err = db.Exec(`
		CREATE TABLE photos (
			id SERIAL PRIMARY KEY,
			filepath VARCHAR(500) UNIQUE NOT NULL,
			filename VARCHAR(255) NOT NULL,
			folder_id INTEGER REFERENCES folders(id),
			collection VARCHAR(20),
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

	// Create index on folder_id
	_, err = db.Exec(`CREATE INDEX idx_photos_folder_id ON photos(folder_id)`)
	if err != nil {
		log.Fatal(err)
	}

	// Create index on folder path
	_, err = db.Exec(`CREATE INDEX idx_folders_path ON folders(path)`)
	if err != nil {
		log.Fatal(err)
	}

	// Create index on photos filepath
	_, err = db.Exec(`CREATE INDEX idx_photos_filepath ON photos(filepath)`)
	if err != nil {
		log.Fatal(err)
	}

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255),
			name VARCHAR(255),
			google_id VARCHAR(255),
			role VARCHAR(20) DEFAULT 'user',
			approved BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create account_requests table
	_, err = db.Exec(`
		CREATE TABLE account_requests (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255),
			message TEXT,
			status VARCHAR(20) DEFAULT 'pending',
			reviewed_by INTEGER REFERENCES users(id),
			reviewed_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create tags table
	_, err = db.Exec(`
		CREATE TABLE tags (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create photo_tags table (many-to-many)
	_, err = db.Exec(`
		CREATE TABLE photo_tags (
			id SERIAL PRIMARY KEY,
			photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
			tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		 UNIQUE(photo_id, tag_id)
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create comments table
	_, err = db.Exec(`
		CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			parent_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create proposed_edits table
	_, err = db.Exec(`
		CREATE TABLE proposed_edits (
			id SERIAL PRIMARY KEY,
			photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			field VARCHAR(50) NOT NULL,
			proposed_value TEXT,
			current_value TEXT,
			status VARCHAR(20) DEFAULT 'pending',
			reviewed_by INTEGER REFERENCES users(id),
			reviewed_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create activity_logs table
	_, err = db.Exec(`
		CREATE TABLE activity_logs (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			action VARCHAR(50) NOT NULL,
			entity_type VARCHAR(50),
			entity_id INTEGER,
			details TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create indexes
	_, err = db.Exec(`CREATE INDEX idx_photo_tags_photo_id ON photo_tags(photo_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_photo_tags_tag_id ON photo_tags(tag_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_comments_photo_id ON comments(photo_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_comments_user_id ON comments(user_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_proposed_edits_photo_id ON proposed_edits(photo_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_proposed_edits_status ON proposed_edits(status)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_activity_logs_user_id ON activity_logs(user_id)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE INDEX idx_activity_logs_entity ON activity_logs(entity_type, entity_id)`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Database schema recreated successfully.")
}
