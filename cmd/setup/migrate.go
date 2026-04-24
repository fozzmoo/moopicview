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

	// Add new columns to photos table if they don't exist
	_, err = db.Exec(`
		ALTER TABLE photos
		ADD COLUMN IF NOT EXISTS photo_date DATE,
		ADD COLUMN IF NOT EXISTS date_precision VARCHAR(10) DEFAULT 'unknown',
		ADD COLUMN IF NOT EXISTS date_source VARCHAR(20) DEFAULT 'unknown'
	`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Photos table schema updated successfully.")
}
