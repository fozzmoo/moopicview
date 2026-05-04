package main

import (
	"database/sql"
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

	// Drop and recreate photos table
	_, err = db.Exec(`DROP TABLE IF EXISTS photos CASCADE`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE photos (
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

	log.Println("Photos table dropped and recreated successfully.")
}
