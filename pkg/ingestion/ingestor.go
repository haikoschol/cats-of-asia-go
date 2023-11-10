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

package ingestion

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	coa "github.com/haikoschol/cats-of-asia"
	_ "github.com/joho/godotenv/autoload"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"googlemaps.github.io/maps"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime"
	"net/url"
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
)

type Ingestor struct {
	db       coa.Database
	gmaps    *maps.Client
	gdrive   *drive.Service
	folderID string
	logger   func(format string, v ...any)
	verbose  bool
}

type Logger func(string, ...any)

type GoogleCredentials struct {
	MapsAPIKey           string
	SvcAccountEmail      string
	SvcAccountPrivateKey string
}

func NewIngestor(
	db coa.Database,
	credentials GoogleCredentials,
	folderID string,
	logger Logger,
	verbose bool,
) (*Ingestor, error) {

	gmaps, err := maps.NewClient(maps.WithAPIKey(credentials.MapsAPIKey))
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate Google Maps client: %w", err)
	}

	config := &jwt.Config{
		Email:      credentials.SvcAccountEmail,
		PrivateKey: []byte(credentials.SvcAccountPrivateKey),
		TokenURL:   google.JWTTokenURL,
		Scopes:     []string{drive.DriveScope},
	}

	ctx := context.Background()
	client := config.Client(ctx)
	gdrive, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Google Drive service: %w", err)
	}

	return &Ingestor{
		db,
		gmaps,
		gdrive,
		folderID,
		logger,
		verbose,
	}, nil
}

func (i *Ingestor) IngestDirectory(dir string) ([]coa.Image, error) {
	images, err := i.collectFileInfo(dir)
	if err != nil {
		return nil, err
	}

	images, err = i.db.RemoveKnownImages(images)
	if err != nil {
		return nil, err
	}

	if len(images) == 0 && i.verbose {
		i.logger("no new images found at %s\n", dir)
		return images, nil
	}

	images, err = i.resizeImages(images)
	if err != nil {
		return nil, fmt.Errorf("error while resizing images: %w", err)
	}

	// This needs to happen before fixing timezones and geocoding, to avoid redundant requests to the Google Maps API.
	images, err = i.setCoordinateID(images)
	if err != nil && i.verbose {
		i.logger("unable to add existing locations from DB: %v\n", err)
	}

	images, err = i.fixTimezones(images)
	if err != nil {
		return nil, fmt.Errorf("error while fixing timezones: %w", err)
	}

	images, err = i.reverseGeocode(images)
	if err != nil {
		return nil, fmt.Errorf("error while reverse geocoding: %w", err)
	}

	images, err = i.uploadImages(images)
	if err != nil {
		return nil, fmt.Errorf("error while uploading files to Google Drive: %w", err)
	}

	err = i.insertImages(images)
	if err != nil {
		return nil, fmt.Errorf("error while inserting new images into db: %w", err)
	}

	return images, nil
}

func (i *Ingestor) collectFileInfo(dir string) ([]coa.Image, error) {
	if i.verbose {
		i.logger("scanning directory %s...", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("os.ReadDir(%s): %w", dir, err)
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
			i.close(f)
			return nil, fmt.Errorf("unable to calculate SHA256 checksum of file at %s: %w", abspath, err)
		}

		hash := fmt.Sprintf("%x", h.Sum(nil))
		if _, err := f.Seek(0, 0); err != nil {
			i.close(f)
			return nil, fmt.Errorf("unable to seek back to beginning of file at %s: %w", abspath, err)
		}

		exifData, err := exif.Decode(f)
		if err != nil {
			i.close(f)
			return nil, fmt.Errorf("unable to decode exif data from file at %s: %w", abspath, err)
		}
		i.close(f)

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

	if i.verbose {
		i.logger("done\n")
	}
	return images, nil
}

// setCoordinateID on images for which the data already exists in the db. This avoids unnecessary requests to the
// Google Maps API.
func (i *Ingestor) setCoordinateID(images []coa.Image) ([]coa.Image, error) {
	var withCoordinateIDs []coa.Image

	for _, img := range images {
		coordID, err := i.db.GetCoordinateID(img.Latitude, img.Longitude)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				withCoordinateIDs = append(withCoordinateIDs, img)
				continue
			}
			return nil, err
		}

		img.CoordinateID = &coordID
		withCoordinateIDs = append(withCoordinateIDs, img)
	}
	return withCoordinateIDs, nil
}

func (i *Ingestor) fixTimezones(images []coa.Image) ([]coa.Image, error) {
	if i.verbose {
		i.logger("fixing timezones...\n")
	}

	var fixed []coa.Image

	for _, img := range images {
		fixedImg := img

		if img.CoordinateID != nil {
			if i.verbose {
				i.logger("coordinate ID already set for image %s. skipping\n", img.PathLarge)
			}
			fixed = append(fixed, fixedImg)
			continue
		}

		tzID, err := i.getTimezoneID(img.Timestamp, img.Latitude, img.Longitude)
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

	if i.verbose {
		i.logger("done\n")
	}
	return fixed, nil
}

func (i *Ingestor) getTimezoneID(t time.Time, lat float64, lng float64) (*time.Location, error) {
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

	res, err := i.gmaps.Timezone(context.Background(), &req)
	if err != nil {
		return nil, err
	}

	return time.LoadLocation(res.TimeZoneID)
}

func (i *Ingestor) reverseGeocode(images []coa.Image) ([]coa.Image, error) {
	if i.verbose {
		i.logger("reverse geocoding...\n")
	}

	var geocoded []coa.Image

	for _, img := range images {
		imgWithLoc := img

		if img.CoordinateID != nil {
			if i.verbose {
				i.logger("coordinate ID already set for image %s. skipping\n", img.PathLarge)
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

		locs, err := i.gmaps.ReverseGeocode(context.Background(), r)
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

	if i.verbose {
		i.logger("done\n")
	}
	return geocoded, nil
}

func (i *Ingestor) insertImages(images []coa.Image) error {
	if i.verbose {
		i.logger("inserting images into db...\n")
	}

	if err := i.db.InsertImages(images); err != nil {
		return err
	}

	if i.verbose {
		i.logger("done\n")
	}
	return nil
}

func (i *Ingestor) close(c io.Closer) {
	if err := c.Close(); err != nil && i.verbose {
		i.logger("Close() failed: %v\n", err)
	}
}

func (i *Ingestor) resizeImages(images []coa.Image) ([]coa.Image, error) {
	if i.verbose {
		i.logger("resizing images...\n")
	}

	var resized []coa.Image

	for _, img := range images {
		imgWithResized := img
		var err error

		imgWithResized.PathSmall, err = i.resizeImage(img.PathLarge, imageSuffixSmall, imageWidthSmall)
		if err != nil {
			return nil, err
		}

		imgWithResized.PathMedium, err = i.resizeImage(img.PathLarge, imageSuffixMedium, imageWidthMedium)
		if err != nil {
			return nil, err
		}

		resized = append(resized, imgWithResized)
	}

	if i.verbose {
		i.logger("done\n")
	}
	return resized, nil
}

func (i *Ingestor) resizeImage(path, suffix string, width int) (string, error) {
	dir := filepath.Dir(path)
	basename := filepath.Base(path)
	ext := filepath.Ext(path)
	withoutExt := strings.TrimSuffix(basename, ext)
	pathResized := filepath.Join(dir, fmt.Sprintf("%s%s%s", withoutExt, suffix, ext))

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
		if i.verbose {
			i.logger("resized image file %s already exists\n", pathResized)
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

func (i *Ingestor) uploadImages(images []coa.Image) ([]coa.Image, error) {
	var withURLs []coa.Image

	if i.verbose {
		i.logger("uploading %d images to Google Drive...\n", len(images))
	}

	for _, img := range images {
		imgWithURLs := img
		var err error

		imgWithURLs.URLLarge, err = i.uploadFile(imgWithURLs.PathLarge)
		if err != nil {
			return nil, err
		}

		imgWithURLs.URLMedium, err = i.uploadFile(imgWithURLs.PathMedium)
		if err != nil {
			return nil, err
		}

		imgWithURLs.URLSmall, err = i.uploadFile(imgWithURLs.PathSmall)
		if err != nil {
			return nil, err
		}

		withURLs = append(withURLs, imgWithURLs)
	}

	if i.verbose {
		i.logger("done\n")
	}

	return withURLs, nil
}

// uploadFile uploads a local file at path to Google Drive and returns the URL to the file.
func (i *Ingestor) uploadFile(path string) (*url.URL, error) {
	src, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open file %s: %w", path, err)
	}
	defer i.close(src)

	dst := &drive.File{
		Name:    filepath.Base(path),
		Parents: []string{i.folderID},
	}

	res, err := i.createGDriveFile(path, src, dst)
	if err != nil {
		return nil, fmt.Errorf("unable to upload file %s to Google Drive folder %s: %w", path, i.folderID, err)
	}

	wcl, err := url.Parse(res.WebContentLink)
	if err != nil {
		return nil, fmt.Errorf("unable to parse Google Drive file URL %s: %w", res.WebContentLink, err)
	}

	// Only keep the "id" query parameter. The API also returns at least "export=download" which causes the browser
	// to download the image instead of displaying it.
	q := wcl.Query()
	wcl.RawQuery = fmt.Sprintf("id=%s", q.Get("id"))

	return wcl, nil
}

func (i *Ingestor) createGDriveFile(path string, src *os.File, dest *drive.File) (*drive.File, error) {
	return i.gdrive.Files.Create(dest).
		Media(src, googleapi.ContentType(mime.TypeByExtension(strings.ToLower(filepath.Ext(path))))).
		Fields("webContentLink").
		Do()
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
