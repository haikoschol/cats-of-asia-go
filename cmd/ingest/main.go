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
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/haikoschol/cats-of-asia/pkg/postgres"
	_ "github.com/joho/godotenv/autoload"
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
	dbSSLMode  = os.Getenv("COA_DB_SSLMODE")
	dbName     = os.Getenv("COA_DB_NAME")
	dbUser     = os.Getenv("COA_DB_USER")
	dbPassword = os.Getenv("COA_DB_PASSWORD")

	googleMapsApiKey = os.Getenv("COA_GOOGLE_MAPS_API_KEY")
)

func main() {
	validateEnv()

	db, err := postgres.NewDatabase(dbUser, dbPassword, dbHost, dbName, postgres.SSLMode(dbSSLMode))
	gmapsClient, err := maps.NewClient(maps.WithAPIKey(googleMapsApiKey))
	if err != nil {
		log.Fatalf("unable to instantiate Google Maps client: %v\n", err)
	}

	ingestDirectory(getImageDir(), gmapsClient, db)
}

func ingestDirectory(dir string, mapsClient *maps.Client, db coa.Database) {
	images, err := collectFileInfo(dir)
	if err != nil {
		log.Fatalf("error while reading image files: %v\n", err)
	}

	images, err = db.RemoveKnownImages(images)
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

	// This needs to happen before fixing timezones and geocoding, to avoid redundant requests to the Google Maps API.
	images, err = setCoordinateID(images, db)
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

func collectFileInfo(dir string) ([]coa.Image, error) {
	if verbose {
		log.Printf("scanning directory %s...", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("os.ReadDir(): %v\n", err)
	}

	var images []coa.Image

	for _, entry := range entries {
		name := entry.Name()
		if !coa.IsSupportedMedia(name) {
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

		img := coa.Image{
			PathLarge: abspath,
			SHA256:    hash,
			Latitude:  latitude,
			Longitude: longitude,
			Timestamp: creationTime,
		}

		images = append(images, img)
	}

	if verbose {
		log.Println("done")
	}
	return images, nil
}

// SetCoordinateID on images for which the data already exists in the db. This avoids unnecessary requests to the
// Google Maps API.
func setCoordinateID(images []coa.Image, db coa.Database) ([]coa.Image, error) {
	var withCoordinateIDs []coa.Image

	for _, img := range images {
		coordID, err := db.GetCoordinateID(img.Latitude, img.Longitude)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}

		img.CoordinateID = &coordID

		withCoordinateIDs = append(withCoordinateIDs, img)
	}

	return withCoordinateIDs, nil
}

func fixTimezones(images []coa.Image, client *maps.Client) ([]coa.Image, error) {
	if verbose {
		log.Println("fixing timezones...")
	}

	var fixed []coa.Image

	for _, img := range images {
		fixedImg := img

		if img.CoordinateID != nil {
			if verbose {
				log.Printf("coordinate ID already set for image %s. skipping\n", img.PathLarge)
			}
			fixed = append(fixed, fixedImg)
			continue
		}

		tzID, err := getTimezoneID(img.Timestamp, img.Latitude, img.Longitude, client)
		if err != nil {
			return nil, err
		}

		localTS, err := time.ParseInLocation(time.DateTime, img.Timestamp.Format(time.DateTime), tzID)
		if err != nil {
			return nil, err
		}

		fixedImg.Timestamp = localTS.UTC()
		fixedImg.Timezone = tzID.String()
		fixed = append(fixed, fixedImg)
	}

	if verbose {
		log.Println("done")
	}
	return fixed, nil
}

func getTimezoneID(t time.Time, lat float64, lng float64, client *maps.Client) (*time.Location, error) {
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

func reverseGeocode(images []coa.Image, client *maps.Client) ([]coa.Image, error) {
	if verbose {
		log.Println("reverse geocoding...")
	}

	var geocoded []coa.Image

	for _, img := range images {
		imgWithLoc := img

		if img.CoordinateID != nil {
			if verbose {
				log.Printf("coordinate ID already set for image %s. skipping\n", img.PathLarge)
			}
			geocoded = append(geocoded, imgWithLoc)
			continue
		}

		r := &maps.GeocodingRequest{
			LatLng: &maps.LatLng{
				Lat: img.Latitude,
				Lng: img.Longitude,
			},
		}

		locs, err := client.ReverseGeocode(context.Background(), r)
		if err != nil {
			return nil, err
		}

		if len(locs) == 0 || len(locs[0].AddressComponents) == 0 {
			return nil, fmt.Errorf(
				"the Google Maps API did not return required address components for latitude %f, longitude %f",
				img.Latitude,
				img.Longitude,
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
						imgWithLoc.City = "Bangkok"
					case "เชียงใหม่":
						imgWithLoc.City = "Chang Wat Chiang Mai"
					case "Chang Wat Samut Prakan":
						imgWithLoc.City = "Samut Prakan"
					case "Wilayah Persekutuan Kuala Lumpur":
						imgWithLoc.City = "Kuala Lumpur"
					default:
						imgWithLoc.City = comp.LongName
					}
				} else if t == "country" {
					imgWithLoc.Country = comp.LongName
					if comp.LongName == "Taiwan" {
						imgWithLoc.City = neighborhood
					}
				}
				if imgWithLoc.City != "" && imgWithLoc.Country != "" {
					break
				}
			}
		}

		if imgWithLoc.City == "" || imgWithLoc.Country == "" {
			return nil, fmt.Errorf("couldn't find either city or country for coordinates %f, %f", img.Latitude, img.Longitude)
		}

		geocoded = append(geocoded, imgWithLoc)
	}

	if verbose {
		log.Println("done")
	}
	return geocoded, nil
}

func resizeImages(images []coa.Image) ([]coa.Image, error) {
	if verbose {
		log.Println("resizing images...")
	}

	var resized []coa.Image

	for _, img := range images {
		imgWithResized := img
		var err error

		imgWithResized.PathSmall, err = resizeImage(img.PathLarge, imageSuffixSmall, imageWidthSmall)
		if err != nil {
			return nil, err
		}

		imgWithResized.PathMedium, err = resizeImage(img.PathLarge, imageSuffixMedium, imageWidthMedium)
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

func insertImages(images []coa.Image, db coa.Database) error {
	if verbose {
		log.Println("inserting images into db...")
	}

	if err := db.InsertImages(images); err != nil {
		return err
	}

	if verbose {
		log.Println("done")
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
	if dbSSLMode == "" {
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
