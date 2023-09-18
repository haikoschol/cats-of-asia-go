// Copyright (C) 2023 Haiko Schol
// SPDX-License-Identifier: GPL-3.0-or-later

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq"
	"log"
	"os"
	"time"
)

var (
	dbHost     = os.Getenv("COA_DB_HOST")
	dbSSLmode  = os.Getenv("COA_DB_SSLMODE")
	dbName     = os.Getenv("COA_DB_NAME")
	dbUser     = os.Getenv("COA_DB_USER")
	dbPassword = os.Getenv("COA_DB_PASSWORD")
)

type stateJSONFile struct {
	Path       string
	MediaItems map[string]bool
}

func main() {
	validateEnv()

	dbURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", dbUser, dbPassword, dbHost, dbName, dbSSLmode)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}

	if len(os.Args) != 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("usage: %s <state file>\n", os.Args[0])
		os.Exit(1)
	}

	now := time.Now()
	state := parseStateFile(os.Args[1])

	platformId, err := getPlatformId("Mastodon", db)
	if err != nil {
		log.Fatalf("unable to retrieve platform ID for Mastodon from the database: %v\n", err)
	}

	for path, _ := range state.MediaItems {
		if err := insertPost(path, now, platformId, db); err != nil {
			log.Fatalf("unable to insert post for image %s: %v\n", path, err)
		}
	}
}

func insertPost(imgPath string, timestamp time.Time, platformId int64, db *sql.DB) error {
	row := db.QueryRow("SELECT id FROM images WHERE path_large = $1", imgPath)

	var imgId int64
	err := row.Scan(&imgId)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		"INSERT INTO posts(image_id, platform_id, timestamp) VALUES ($1, $2, $3)",
		imgId,
		platformId,
		timestamp.UTC())
	if err != nil {
		return err
	}

	return nil
}

func getPlatformId(name string, db *sql.DB) (int64, error) {
	row := db.QueryRow("SELECT id FROM platforms WHERE name = $1", name)

	var id int64
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func parseStateFile(path string) stateJSONFile {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("unable to read state file: %v\n", err)
	}

	var state stateJSONFile
	if err := json.Unmarshal(data, &state); err != nil {
		log.Fatalf("unable to parse state file as JSON: %v\n", err)
	}

	return state
}

func validateEnv() {
	var errors []string

	if dbHost == "" {
		errors = append(errors, "COA_DB_HOST env var missing")
	}
	if dbSSLmode == "" {
		errors = append(errors, "COA_DB_SSLMODE env var missing")
	}
	if dbName == "" {
		errors = append(errors, "COA_DB_NAME env var missing")
	}
	if dbUser == "" {
		errors = append(errors, "COA_DB_USER env var missing")
	}
	if dbPassword == "" {
		errors = append(errors, "COA_DB_PASSWORD env var missing")
	}

	for _, e := range errors {
		fmt.Println(e)
	}

	if len(errors) > 0 {
		os.Exit(1)
	}
}
