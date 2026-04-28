package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://moopicview:moopicview123@localhost:7432/moopicview?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer db.Close()

	// Check if photo 20440 exists
	var id int
	err = db.QueryRow("SELECT id FROM photos WHERE id = $1", 20440).Scan(&id)
	if err != nil {
		fmt.Printf("Photo 20440 not found: %v\n", err)
	} else {
		fmt.Printf("Photo 20440 found: ID=%d\n", id)
	}

	// Check total count
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM photos").Scan(&count)
	if err != nil {
		fmt.Printf("Failed to count: %v\n", err)
	} else {
		fmt.Printf("Total photos: %d\n", count)
	}

	// Check highest ID
	var maxID int
	err = db.QueryRow("SELECT MAX(id) FROM photos").Scan(&maxID)
	if err != nil {
		fmt.Printf("Failed to get max ID: %v\n", err)
	} else {
		fmt.Printf("Max ID: %d\n", maxID)
	}
}
