package main

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
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

	// Get PHOTO_ROOTS from environment
	rootsStr := os.Getenv("PHOTO_ROOTS")
	if rootsStr == "" {
		rootsStr = "digital:/unas/images/digital_photos/2017/20170625-FortBuenaVentura,scanned:/unas/images/scanned_photos/scan-date/2024/20240404"
	}

	rootEntries := strings.Split(rootsStr, ",")

	// First, scan and create all folder entries
	folderMap := make(map[string]int)
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

		// Scan all directories under this root
		scanAndCreateFolders(db, path, collectionType, folderMap)
	}

	fmt.Println("\nScanning and importing photos...")
	// Then, scan and import photos
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

		scanAndImportPhotos(db, path, collectionType, folderMap)
	}

	fmt.Println("\nScan complete.")
}

func scanAndCreateFolders(db *sql.DB, rootPath string, collectionType string, folderMap map[string]int) {
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		// Get the directory name
		name := filepath.Base(path)
		parentPath := filepath.Dir(path)

		// Insert the folder
		var folderID int
		err = db.QueryRow(`
			INSERT INTO folders (path, name, parent_path, collection_type)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, path, name, parentPath, collectionType).Scan(&folderID)

		if err != nil {
			log.Printf("Error inserting folder %s: %v", path, err)
			return nil
		}

		folderMap[path] = folderID
		fmt.Printf("Created folder: %s (ID: %d, Collection: %s)\n", path, folderID, collectionType)

		return nil
	})

	if err != nil {
		log.Printf("Error scanning %s: %v", rootPath, err)
	}
}

func scanAndImportPhotos(db *sql.DB, rootPath string, collectionType string, folderMap map[string]int) {
	err := filepath.WalkDir(rootPath, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		nameLower := strings.ToLower(name)
		if !strings.HasSuffix(nameLower, ".jpg") && !strings.HasSuffix(nameLower, ".jpeg") && !strings.HasSuffix(nameLower, ".png") {
			return nil
		}

		// Find the folder ID for this photo's directory
		dirPath := filepath.Dir(fullPath)
		folderID := folderMap[dirPath]
		if folderID == 0 {
			// Try parent directories
			parts := strings.Split(dirPath, "/")
			for i := len(parts) - 1; i >= 0; i-- {
				parentPath := strings.Join(parts[:i+1], "/")
				if id, ok := folderMap[parentPath]; ok {
					folderID = id
					break
				}
			}
		}

		// Determine photo date based on type
		var photoDate string
		var datePrecision string = "unknown"
		var dateSource string = "unknown"

		if collectionType == "digital" {
			// Primary: EXIF date
			if date, precision, ok := extractExifDate(fullPath); ok {
				photoDate = date.Format("2006-01-02")
				datePrecision = precision
				dateSource = "exif"
			} else {
				// Fallback: directory name
				parentDir := filepath.Base(filepath.Dir(fullPath))
				if date, precision, source, ok := extractDateFromDirName(parentDir); ok {
					photoDate = date.Format("2006-01-02")
					datePrecision = precision
					dateSource = source
				}
			}
		} else if collectionType == "scanned" {
			// For scanned photos, try to extract date from filename
			if date, precision, source, ok := extractDateFromDirName(name); ok {
				photoDate = date.Format("2006-01-02")
				datePrecision = precision
				dateSource = source
			}
		}

		// Insert photo
		_, err = db.Exec(`
			INSERT INTO photos (filepath, filename, folder_id, collection, scan_date, photo_date, date_precision, date_source, description)
			VALUES ($1, $2, $3, $4, CURRENT_DATE, $5, $6, $7, $8)
		`, fullPath, name, folderID, collectionType, photoDate, datePrecision, dateSource, "Scanned photo")

		if err == nil {
			log.Printf("Imported photo: %s (Folder ID: %d, Date: %v)\n", name, folderID, photoDate)
		} else {
			log.Printf("Error importing %s: %v\n", fullPath, err)
		}

		return nil
	})

	if err != nil {
		log.Printf("Error scanning %s: %v", rootPath, err)
	}
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
