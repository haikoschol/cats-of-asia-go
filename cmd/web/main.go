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
	"embed"
	"encoding/json"
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	dbHost            = os.Getenv("COA_DB_HOST")
	dbSSLmode         = os.Getenv("COA_DB_SSLMODE")
	dbName            = os.Getenv("COA_DB_NAME")
	dbUser            = os.Getenv("COA_DB_USER")
	dbPassword        = os.Getenv("COA_DB_PASSWORD")
	mapboxAccessToken = os.Getenv("COA_MAPBOX_ACCESS_TOKEN")

	//go:embed "static"
	staticEmbed embed.FS
	staticFs    http.FileSystem = http.FS(staticEmbed)

	//go:embed "templates/index.html"
	indexHTML     string
	indexTemplate = template.Must(template.New("cattos").Parse(indexHTML))
)

type image struct {
	ID         int64     `json:"id"`
	Path       string    `json:"path"`
	Timestamp  time.Time `json:"timestamp"`
	tzLocation string
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	City       string  `json:"city"`
	Country    string  `json:"country"`
}

func main() {
	if mapboxAccessToken == "" {
		log.Fatal("env var COA_MAPBOX_ACCESS_TOKEN not set")
	}

	api, err := newWebApp(dbUser, dbPassword, dbHost, dbName, dbSSLmode)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/images", api.handleImages)
	mux.HandleFunc("/images/", api.handleGetImage)

	mux.Handle("/static/", http.FileServer(staticFs))
	mux.HandleFunc("/", api.handleIndex)

	log.Print("Starting server on :4000")
	log.Fatal(http.ListenAndServe(":4000", mux))
}

func (app *webApp) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path != "/" {
		serve404(w)
		return
	}

	data := map[string]interface{}{
		"access_token": mapboxAccessToken,
	}

	w.Header().Add("Content-Type", "text/html")

	if err := indexTemplate.Execute(w, data); err != nil {
		log.Println("failed to render index template:", err)
		return
	}
}

func (app *webApp) handleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	images, err := fetchImages(app.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	b, err := json.Marshal(images)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if _, err := w.Write(b); err != nil {
		log.Println("failed writing http response:", err)
	}
}

func (app *webApp) handleGetImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id, found := strings.CutPrefix(r.URL.Path, "/images/")
	if !found || id == "" {
		http.Redirect(w, r, "/images", http.StatusMovedPermanently)
		return
	}

	row := app.db.QueryRow(`SELECT path FROM images WHERE id = $1;`, id)
	var imgPath string
	if err := row.Scan(&imgPath); err != nil {
		// TODO send 404 if row not found
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	stats, err := os.Stat(imgPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("unable to stat file at %s: %w", imgPath, err))
		return
	}

	f, err := os.Open(imgPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("unable to open file at %s: %w", imgPath, err))
		return
	}
	defer f.Close()

	w.Header().Add("Content-Type", "image/jpeg") // TODO support more image formats and video
	w.Header().Add("Content-Length", fmt.Sprintf("%d", stats.Size()))

	if _, err := io.Copy(w, f); err != nil {
		log.Println("failed sending image in http response:", err)
		return
	}
}

type webApp struct {
	db *sql.DB
}

func newWebApp(dbUser, dbPassword, dbHost, dbName, dbSSLmode string) (*webApp, error) {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", dbUser, dbPassword, dbHost, dbName, dbSSLmode)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	return &webApp{db}, nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)

	payload := map[string]string{"error": fmt.Sprintf("%v", err)}
	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("unable to serialize error payload:", err)
		return
	}

	if _, err := w.Write(b); err != nil {
		log.Println("failed writing http error response:", err)
	}
}

// TODO send html with the image instead and include credit and link to https://http.cat/
func serve404(w http.ResponseWriter) {
	f, err := staticEmbed.Open("static/404.jpg")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer f.Close()

	w.Header().Add("Content-Type", "image/jpeg") // TODO support more image formats and video

	if _, err := io.Copy(w, f); err != nil {
		log.Println("failed sending image in http response:", err)
		return
	}
}

func fetchImages(db *sql.DB) ([]image, error) {
	query := `SELECT 
		i.id,
		i.path,
		i.timestamp,
		i.tz_location,
		i.latitude,
		i.longitude,
		l.city,
		l.country
	FROM images AS i
	LEFT JOIN locations AS l ON i.id = l.image_id;`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}

	var images []image
	for rows.Next() {
		var img image
		err := rows.Scan(&img.ID, &img.Path, &img.Timestamp, &img.tzLocation, &img.Latitude, &img.Longitude, &img.City, &img.Country)
		if err != nil {
			return nil, err
		}

		loc, err := time.LoadLocation(img.tzLocation)
		if err != nil {
			return nil, err
		}

		img.Timestamp = img.Timestamp.In(loc)
		images = append(images, img)
	}

	return images, rows.Err()
}
