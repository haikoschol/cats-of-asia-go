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

package coa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

type Image struct {
	ID           int64
	CoordinateID *int64
	PathLarge    string
	PathMedium   string
	PathSmall    string
	URLLarge     *url.URL
	URLMedium    *url.URL
	URLSmall     *url.URL
	SHA256       string
	Timestamp    time.Time
	Timezone     string
	Latitude     float64
	Longitude    float64
	City         string
	Country      string
}

func (img Image) Path() string {
	return img.PathLarge
}

func (img Image) Read() (io.ReadCloser, error) {
	f, err := os.Open(img.Path())
	if err != nil {
		return nil, fmt.Errorf("unable to open file at %s: %w", img.Path(), err)
	}
	return f, err
}

func (img Image) Content() ([]byte, error) {
	data, err := os.ReadFile(img.Path())
	if err != nil {
		return nil, fmt.Errorf("unable to read file at %s: %w", img.Path(), err)
	}
	return data, nil
}

func (img Image) Location() string {
	if img.City == "" && img.Country != "" {
		return img.Country
	}
	if img.Country == "" && img.City != "" {
		return img.City
	}
	if img.City == "" && img.Country == "" {
		return "an undisclosed location"
	}

	return fmt.Sprintf("%s, %s", img.City, img.Country)
}

func (img Image) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID        int64     `json:"id"`
		URLLarge  string    `json:"urlLarge"`
		URLMedium string    `json:"urlMedium"`
		URLSmall  string    `json:"urlSmall"`
		SHA256    string    `json:"sha256"`
		Timestamp time.Time `json:"timestamp"`
		Latitude  float64   `json:"latitude"`
		Longitude float64   `json:"longitude"`
		City      string    `json:"city"`
		Country   string    `json:"country"`
	}{
		ID:        img.ID,
		URLLarge:  img.URLLarge.String(),
		URLMedium: img.URLMedium.String(),
		URLSmall:  img.URLSmall.String(),
		SHA256:    img.SHA256,
		Timestamp: img.Timestamp,
		Latitude:  img.Latitude,
		Longitude: img.Longitude,
		City:      img.City,
		Country:   img.Country,
	})
}

type Platform string

const (
	Mastodon Platform = "Mastodon"
	X                 = "X"
)

type Database interface {
	GetOrCreateLocation(city, country, timezone string) (int64, error)
	GetOrCreateCoordinates(latitude, longitude float64, locationId int64) (int64, error)
	GetCoordinateID(latitude, longitude float64) (int64, error)
	GetImage(id int64) (Image, error)
	GetImages() ([]Image, error)
	GetRandomUnusedImage(platform Platform) (Image, error)
	GetUnusedImageCount(platform Platform) (int, error)
	RemoveKnownImages(images []Image) ([]Image, error)
	InsertImages(images []Image) error
	InsertPost(image Image, platform Platform) error
}

// Publisher allows posting images to a platform.
type Publisher interface {
	// Platform returns the platform a Publisher instance posts to.
	Platform() Platform
	// Publish sends an image together with a description to a platform.
	Publish(image Image, description string) error
}

// IsSupportedMedia checks whether a given file type can be used by the bot/web app (JPEG only for now)
func IsSupportedMedia(filename string) bool {
	filename = strings.ToLower(filename)
	return strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg")
}
