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
	"errors"
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	_ "github.com/joho/godotenv/autoload"
	"github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	"googlemaps.github.io/maps"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	imageWidthSmall   = 300
	imageWidthMedium  = 600
	imageSuffixSmall  = "-small"
	imageSuffixMedium = "-medium"
	verbose           = true // TODO make cli flag
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
	pathLarge    string
	pathMedium   string
	pathSmall    string
	sha256       string
	coordinateId *int64
	tzLocation   string
	timestamp    time.Time
	latitude     float64
	longitude    float64
	city         string
	country      string
}

func main() {
	validateEnv()

	dbURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", dbUser, dbPassword, dbHost, dbName, dbSSLmode)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}

	mapsClient, err := maps.NewClient(maps.WithAPIKey(googleMapsApiKey))
	if err != nil {
		log.Fatalf("unable to instantiate Google Maps client: %v\n", err)
	}

	ingestDirectory(getImageDir(), mapsClient, db)
}

func ingestDirectory(dir string, mapsClient *maps.Client, db *sql.DB) {
	images, err := collectFileInfo(dir)
	if err != nil {
		log.Fatalf("error while reading image files: %v\n", err)
	}

	images, err = removeKnownImages(images, db)
	if err != nil {
		log.Fatal(err)
	}

	if len(images) == 0 {
		log.Printf("no new images found at %s\n", dir)
		os.Exit(0)
	}

	images, err = resizeImages(images)
	if err != nil {
		log.Fatalf("error while resizing images: %v\n", err)
	}

	images, err = addLocationsFromDb(images, db)
	if err != nil {
		log.Printf("unable to add existing locations from DB: %v\n", err)
	}

	images, err = fixTimezones(images, mapsClient)
	if err != nil {
		log.Fatalf("error while fixing timezones: %v\n", err)
	}

	images, err = reverseGeocode(images, mapsClient)
	if err != nil {
		log.Fatalf("error while reverse geocoding: %v\n", err)
	}

	err = insertImages(images, db)
	if err != nil {
		log.Fatalf("error while inserting new images into db: %v\n", err)
	}
}

func collectFileInfo(dir string) ([]imageWithLoc, error) {
	if verbose {
		log.Printf("scanning directory %s...", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("os.ReadDir(): %v\n", err)
	}

	var images []imageWithLoc

	for _, entry := range entries {
		name := entry.Name()
		if !coabot.IsSupportedMedia(name) {
			continue
		}

		// skip resized images that may have been created in a previous run
		basename := strings.TrimSuffix(name, filepath.Ext(name))
		if strings.HasSuffix(basename, imageSuffixSmall) || strings.HasSuffix(basename, imageSuffixMedium) {
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
			pathLarge:  abspath,
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

	if verbose {
		log.Println("done")
	}
	return images, nil
}

func removeKnownImages(images []imageWithLoc, db *sql.DB) ([]imageWithLoc, error) {
	var hashes []string

	for _, img := range images {
		hashes = append(hashes, img.sha256)
	}

	rows, err := db.Query(`SELECT path_large, sha256 FROM images WHERE sha256 = ANY($1)`, pq.Array(hashes))
	if err != nil {
		return nil, err
	}

	knownImages := make(map[string]string)

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
			if verbose {
				log.Printf("file %s already exists in the database as %s\n", img.pathLarge, imgPath)
			}
			continue
		}

		filtered = append(filtered, img)
	}

	return filtered, nil
}

// Terrible name. The function sets coordinateId and locationId in the imageWithLoc structs for which the data already
// exists in the db. Later on those values will be used in the "INSERT images" query and no new coordinates or locations
// need to be inserted.
func addLocationsFromDb(images []imageWithLoc, db *sql.DB) ([]imageWithLoc, error) {
	var withLocations []imageWithLoc

	for _, img := range images {
		withLoc := img

		row := db.QueryRow(
			"SELECT id from coordinates WHERE latitude = $1 AND longitude = $2",
			img.latitude,
			img.longitude,
		)

		var id int64
		err := row.Scan(&id)
		if err == nil {
			withLoc.coordinateId = &id
		} else if !errors.Is(err, sql.ErrNoRows) {
			log.Printf(
				"error while fetching coordinate ID for image %s. latitude: %f longitude: %f error: %v\n",
				img.pathLarge,
				img.latitude,
				img.longitude,
				err,
			)
		}

		withLocations = append(withLocations, withLoc)
	}

	return withLocations, nil
}

func fixTimezones(images []imageWithLoc, client *maps.Client) ([]imageWithLoc, error) {
	if verbose {
		log.Println("fixing timezones...")
	}

	var fixed []imageWithLoc

	for _, img := range images {
		fixedImg := img

		if img.coordinateId != nil {
			if verbose {
				log.Printf("coordinate ID already set for image %s. skipping\n", img.pathLarge)
			}
			fixed = append(fixed, fixedImg)
			continue
		}

		loc, err := getLocation(img.timestamp, img.latitude, img.longitude, client)
		if err != nil {
			return nil, err
		}

		localTS, err := time.ParseInLocation(time.DateTime, img.timestamp.Format(time.DateTime), loc)
		if err != nil {
			return nil, err
		}

		fixedImg.timestamp = localTS.UTC()
		fixedImg.tzLocation = loc.String()
		fixed = append(fixed, fixedImg)
	}

	if verbose {
		log.Println("done")
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
	if verbose {
		log.Println("reverse geocoding...")
	}

	var geocoded []imageWithLoc

	for _, img := range images {
		imgWithLoc := img

		if img.coordinateId != nil {
			if verbose {
				log.Printf("coordinate ID already set for image %s. skipping\n", img.pathLarge)
			}
			geocoded = append(geocoded, imgWithLoc)
			continue
		}

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

		var neighborhood string
		for _, comp := range locs[0].AddressComponents {
			for _, t := range comp.Types {
				if t == "neighborhood" {
					neighborhood = comp.LongName
				} else if t == "administrative_area_level_1" {
					switch comp.LongName {
					case "กรุงเทพมหานคร":
						imgWithLoc.city = "Bangkok"
					case "เชียงใหม่":
						imgWithLoc.city = "Chang Wat Chiang Mai"
					case "Chang Wat Samut Prakan":
						imgWithLoc.city = "Samut Prakan"
					case "Wilayah Persekutuan Kuala Lumpur":
						imgWithLoc.city = "Kuala Lumpur"
					default:
						imgWithLoc.city = comp.LongName
					}
				} else if t == "country" {
					imgWithLoc.country = comp.LongName
					if comp.LongName == "Taiwan" {
						imgWithLoc.city = neighborhood
					}
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

	if verbose {
		log.Println("done")
	}
	return geocoded, nil
}

func resizeImages(images []imageWithLoc) ([]imageWithLoc, error) {
	if verbose {
		log.Println("resizing images...")
	}

	var resized []imageWithLoc

	for _, img := range images {
		imgWithResized := img
		var err error

		imgWithResized.pathSmall, err = resizeImage(img.pathLarge, imageSuffixSmall, imageWidthSmall)
		if err != nil {
			return nil, err
		}

		imgWithResized.pathMedium, err = resizeImage(img.pathLarge, imageSuffixMedium, imageWidthMedium)
		if err != nil {
			return nil, err
		}

		resized = append(resized, imgWithResized)
	}

	if verbose {
		log.Println("done")
	}
	return resized, nil
}

func resizeImage(path, suffix string, width int) (string, error) {
	dir := filepath.Dir(path)
	basename := filepath.Base(path)
	ext := filepath.Ext(path)
	withoutExt := strings.TrimSuffix(basename, ext)
	pathResized := filepath.Join(dir, "scaled", fmt.Sprintf("%s%s%s", withoutExt, suffix, ext))

	// make sure the resized file does not exist already and there is no directory with the same name
	stats, err := os.Stat(pathResized)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil && stats.IsDir() {
		return "", fmt.Errorf("cannot write resized image to %s. a directory with that name already exists", pathResized)
	}
	// file already exists, nothing to do
	if err == nil {
		if verbose {
			log.Printf("resized image file %s already exists\n", pathResized)
		}
		return pathResized, nil
	}

	src, err := decodeImage(path)
	if err != nil {
		return "", err
	}

	height := src.Bounds().Max.Y / (src.Bounds().Max.X / width)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	if err := encodeImage(dst, pathResized); err != nil {
		return "", err
	}

	return pathResized, nil
}

func decodeImage(path string) (image.Image, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open file %s for resizing: %w", path, err)
	}
	defer func() {
		if err := input.Close(); err != nil {
			log.Printf("error closing file %s: %v\n", path, err)
		}
	}()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg":
		fallthrough
	case ".jpeg":
		return jpeg.Decode(input)
	case ".png":
		return png.Decode(input)
	default:
		return nil, fmt.Errorf("unable to determine image format for decoding %s", path)
	}
}

func encodeImage(m image.Image, path string) error {
	output, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("unable to create file for resized image at %s: %w", path, err)
	}
	defer func() {
		if err := output.Close(); err != nil {
			log.Printf("error closing file %s: %v\n", path, err)
		}
	}()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg":
		fallthrough
	case ".jpeg":
		return jpeg.Encode(output, m, &jpeg.Options{Quality: 100})
	case ".png":
		return png.Encode(output, m)
	default:
		return fmt.Errorf("unable to determine image format for encoding '%s'", path)
	}
}

func insertImages(images []imageWithLoc, db *sql.DB) error {
	if verbose {
		log.Println("inserting images into db...")
	}

	for _, img := range images {
		if img.coordinateId == nil {
			locId, err := getOrCreateLocation(img.city, img.country, img.tzLocation, db)
			if err != nil {
				return err
			}

			coordId, err := getOrCreateCoordinates(img.latitude, img.longitude, locId, db)
			if err != nil {
				return err
			}

			img.coordinateId = &coordId
		}
		if err := insertImage(img, db); err != nil {
			return err
		}
	}

	if verbose {
		log.Println("done")
	}
	return nil
}

func getOrCreateLocation(city, country, timezone string, db *sql.DB) (int64, error) {
	_, err := db.Exec(
		`INSERT INTO
    			locations(city, country, timezone)
			VALUES
			    ($1, $2, $3)
			ON CONFLICT (city, country) DO NOTHING`,
		city,
		country,
		timezone,
	)
	if err != nil {
		return 0, err
	}

	row := db.QueryRow("SELECT id FROM locations WHERE city = $1 and country = $2", city, country)
	var id int64
	err = row.Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func getOrCreateCoordinates(latitude, longitude float64, locationId int64, db *sql.DB) (int64, error) {
	_, err := db.Exec(
		`INSERT INTO
    			coordinates(latitude, longitude, location_id)
			VALUES
			    ($1, $2, $3)
			ON CONFLICT (latitude, longitude) DO NOTHING`,
		latitude,
		longitude,
		locationId,
	)
	if err != nil {
		return 0, err
	}

	row := db.QueryRow("SELECT id FROM coordinates WHERE latitude = $1 and longitude = $2", latitude, longitude)
	var id int64
	err = row.Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func insertImage(img imageWithLoc, db *sql.DB) error {
	_, err := db.Exec(
		`INSERT INTO
    			images(path_large, path_medium, path_small, sha256, timestamp, coordinate_id)
			VALUES
			    ($1, $2, $3, $4, $5, $6)`,
		img.pathLarge,
		img.pathMedium,
		img.pathSmall,
		img.sha256,
		img.timestamp,
		img.coordinateId,
	)
	return err
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
