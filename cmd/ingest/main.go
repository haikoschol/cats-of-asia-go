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
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	_ "github.com/joho/godotenv/autoload"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	"googlemaps.github.io/maps"
	"io"
	"log"
	"os"
	"path"
	"time"
)

var (
	dbHost     = os.Getenv("COA_DB_HOST")
	dbSSLmode  = os.Getenv("COA_DB_SSLMODE")
	dbName     = os.Getenv("COA_DB_NAME")
	dbUser     = os.Getenv("COA_DB_USER")
	dbPassword = os.Getenv("COA_DB_PASSWORD")

	googleMapsApiKey = os.Getenv("COA_GOOGLE_MAPS_API_KEY")
)

type imageWithLoc struct {
	path       string
	sha256     string
	timestamp  time.Time
	tzLocation string
	latitude   float64
	longitude  float64
	city       string
	country    string
}

func main() {
	validateEnv()

	dbURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", dbUser, dbPassword, dbHost, dbName, dbSSLmode)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}

	dir := getImageDir()
	images, err := collectFileInfo(dir)
	if err != nil {
		log.Fatalf("error while reading image files: %v\n", err)
	}

	images, err = removeKnownImages(images, db)
	if err != nil {
		log.Fatal(err)
	}

	mapsClient, err := maps.NewClient(maps.WithAPIKey(googleMapsApiKey))
	if err != nil {
		log.Fatalf("unable to instantiate Google Maps client: %v\n", err)
	}

	images, err = fixTimezones(images, mapsClient)
	if err != nil {
		log.Fatalf("error while fixing timezones: %v\n", err)
	}

	if len(images) == 0 {
		log.Printf("no new images found at %s\n", dir)
		os.Exit(0)
	}

	imagesWithLoc, err := reverseGeocode(images, mapsClient)
	if err != nil {
		log.Fatalf("error while reverse geocoding: %v\n", err)
	}

	err = insertNewImages(imagesWithLoc, db)
	if err != nil {
		log.Fatalf("error while inserting new images into db: %v\n", err)
	}
}

func collectFileInfo(dir string) ([]imageWithLoc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("os.ReadDir(): %v\n", err)
	}

	var images []imageWithLoc

	for _, entry := range entries {
		if !coabot.IsSupportedMedia(entry.Name()) {
			continue
		}

		abspath := path.Join(dir, entry.Name())
		f, err := os.Open(abspath)
		if err != nil {
			return nil, fmt.Errorf("unable to open file at %s: %w", abspath, err)
		}

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return nil, fmt.Errorf("unable to calculate SHA256 checksum of file at %s: %w", abspath, err)
		}

		hash := fmt.Sprintf("%x", h.Sum(nil))
		f.Seek(0, 0)

		exifData, err := exif.Decode(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("unable to decode exif data from file at %s: %w", abspath, err)
		}
		f.Close()

		latitude, longitude, err := exifData.LatLong()
		if err != nil {
			return nil, fmt.Errorf("unable to read GPS coords from exif data in file at %s: %w", abspath, err)
		}

		// Timestamps are assumed to have the wrong timezone, because cameras suck apparently. Will be fixed later.
		creationTime, err := exifData.DateTime()
		if err != nil {
			return nil, fmt.Errorf("unable to read timestamp from  exif data in file at %s: %w", abspath, err)
		}

		image := imageWithLoc{
			path:       abspath,
			sha256:     hash,
			latitude:   latitude,
			longitude:  longitude,
			timestamp:  creationTime,
			tzLocation: "",
			city:       "",
			country:    "",
		}

		images = append(images, image)
	}

	return images, nil
}

func removeKnownImages(images []imageWithLoc, db *sql.DB) ([]imageWithLoc, error) {
	var hashes []string

	for _, img := range images {
		hashes = append(hashes, img.sha256)
	}

	rows, err := db.Query(`SELECT path, sha256 FROM images WHERE sha256 = ANY($1)`, pq.Array(hashes))
	if err != nil {
		return nil, err
	}

	var knownImages map[string]string

	for rows.Next() {
		var imgPath, hash string
		err = rows.Scan(&imgPath, &hash)
		if err != nil {
			return nil, err
		}
		knownImages[hash] = imgPath
	}

	var filtered []imageWithLoc

	for _, img := range images {
		imgPath, ok := knownImages[img.sha256]
		if ok {
			log.Printf("file %s already exists in the database as %s\n", img.path, imgPath)
			continue
		}

		filtered = append(filtered, img)
	}

	return filtered, nil
}

func fixTimezones(images []imageWithLoc, client *maps.Client) ([]imageWithLoc, error) {
	var fixed []imageWithLoc

	for _, img := range images {
		loc, err := getLocation(img.timestamp, img.latitude, img.longitude, client)
		if err != nil {
			return nil, err
		}

		localTS, err := time.ParseInLocation(time.DateTime, img.timestamp.Format(time.DateTime), loc)
		if err != nil {
			return nil, err
		}

		fixedImg := img
		fixedImg.timestamp = localTS.UTC()
		fixedImg.tzLocation = loc.String()
		fixed = append(fixed, fixedImg)
	}

	return fixed, nil
}

func getLocation(t time.Time, lat float64, lng float64, client *maps.Client) (*time.Location, error) {
	t, err := time.ParseInLocation(time.DateTime, t.Format(time.DateTime), time.UTC)
	if err != nil {
		return nil, err
	}

	req := maps.TimezoneRequest{
		Location: &maps.LatLng{
			Lat: lat,
			Lng: lng,
		},
		Timestamp: t,
		Language:  "English",
	}

	res, err := client.Timezone(context.Background(), &req)
	if err != nil {
		return nil, err
	}

	return time.LoadLocation(res.TimeZoneID)
}

func reverseGeocode(images []imageWithLoc, client *maps.Client) ([]imageWithLoc, error) {
	var geocoded []imageWithLoc

	for _, img := range images {
		r := &maps.GeocodingRequest{
			LatLng: &maps.LatLng{
				Lat: img.latitude,
				Lng: img.longitude,
			},
		}

		locs, err := client.ReverseGeocode(context.Background(), r)
		if err != nil {
			return nil, err
		}

		if len(locs) == 0 || len(locs[0].AddressComponents) == 0 {
			return nil, fmt.Errorf(
				"the Google Maps API did not return required address components for latitude %f, longitude %f",
				img.latitude,
				img.longitude,
			)
		}

		imgWithLoc := img
		for _, comp := range locs[0].AddressComponents {
			for _, t := range comp.Types {
				if t == "administrative_area_level_1" {
					imgWithLoc.city = comp.LongName
				} else if t == "country" {
					imgWithLoc.country = comp.LongName
				}
				if imgWithLoc.city != "" && imgWithLoc.country != "" {
					break
				}
			}
		}

		if imgWithLoc.city == "" || imgWithLoc.country == "" {
			return nil, fmt.Errorf("couldn't find either city or country for coordinates %f, %f", img.latitude, img.longitude)
		}

		geocoded = append(geocoded, imgWithLoc)
	}

	return geocoded, nil
}

func insertNewImages(images []imageWithLoc, db *sql.DB) error {
	for _, img := range images {
		tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return err
		}

		row := tx.QueryRow(
			`INSERT INTO images(path, sha256, latitude, longitude, timestamp, tz_location) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			img.path,
			img.sha256,
			img.latitude,
			img.longitude,
			img.timestamp,
			img.tzLocation,
		)

		var imageID int64
		scanErr := row.Scan(&imageID)
		if scanErr != nil {
			_ = tx.Rollback()
			return scanErr
		}

		_, locErr := tx.Exec(
			`INSERT INTO locations(image_id, city, country) VALUES ($1, $2, $3)`,
			imageID,
			img.city,
			img.country,
		)
		if locErr != nil {
			_ = tx.Rollback()
			return locErr
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func getImageDir() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("os.Getcwd(): %v\n", err)
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "--help" || os.Args[1] == "-h" {
			fmt.Printf("usage: %s <path> - directory to scan for new images (default: current directory)\n", os.Args[0])
			os.Exit(1)
		} else {
			dir = os.Args[1]
		}
	}
	return dir
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
	if googleMapsApiKey == "" {
		errors = append(errors, "COA_GOOGLE_MAPS_API_KEY env var missing")
	}

	for _, e := range errors {
		fmt.Println(e)
	}

	if len(errors) > 0 {
		os.Exit(1)
	}
}
