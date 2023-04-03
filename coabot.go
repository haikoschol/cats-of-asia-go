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

package coabot

import (
	"fmt"
	"io"
	"math/rand"
	"time"
)

// MediaCategory denotes whether a media item is a photo or video.
type MediaCategory string
type GibtsNedHammerNedWollnmerNed string

const (
	Photo MediaCategory = "photo"
	Video               = "video"
)

type MediaMetadata struct {
	// CreationTime is the time when the photo or video was taken, in the timezone where it was taken.
	CreationTime time.Time
	Latitude     float64
	Longitude    float64
}

// MediaItem uniquely identifies a photo or video in a MediaAlbum and provides access to metadata and content.
type MediaItem interface {
	// Id is an identifier specific to the MediaAlbum implementation this MediaItem belongs to. It is only guaranteed
	// to be unique in that album.
	Id() string

	// Filename of the photo or video.
	Filename() string

	// Category denotes the type of media.
	Category() MediaCategory

	// Metadata returns info from EXIF tags
	Metadata() (*MediaMetadata, error)

	// Content returns the raw bytes of the photo or video
	Content() ([]byte, error)

	// Read returns an io.ReadCloser for reading the media content. The caller is responsible for closing.
	Read() (io.ReadCloser, error)
}

// MediaAlbum is a repository of media items.
type MediaAlbum interface {
	// Id returns an opaque identifier for the MediaAlbum
	Id() string

	// GetMediaItems lists all media items in the album.
	GetMediaItems() ([]MediaItem, error)
}

// Publisher allows publishing photos or videos.
type Publisher interface {
	// Name returns either "Mastodon" or "Twitter" for now
	Name() string
	// Publish sends a photo or video together with a description to a platform.
	Publish(item MediaItem, description string) error
}

// ApplicationState provides a persistence mechanism for keeping track of which items in a MediaAlbum have already been
// published.
type ApplicationState interface {
	// Add adds a MediaItem to the persistent application state.
	Add(item MediaItem) error

	// Contains checks whether the given MediaItem has already been added to the persistent application state.
	Contains(item MediaItem) bool
}

// Geocoder codes geo
type Geocoder interface {
	// LookupCityAndCountry returns a city and country name for the given coordinates
	LookupCityAndCountry(latitude, longitude float64) (CityAndCountry, error)
}

type CityAndCountry struct {
	City    string
	Country string
}

func (cac CityAndCountry) String() string {
	if cac.City == "" && cac.Country != "" {
		return cac.Country
	}
	if cac.Country == "" && cac.City != "" {
		return cac.City
	}
	if cac.City == "" && cac.Country == "" {
		return "an undisclosed location"
	}

	return fmt.Sprintf("%s, %s", cac.City, cac.Country)
}

// PickRandomUnusedMediaItem returns a random  MediaItem from the given slice, that is not contained in the given
// ApplicationState.
func PickRandomUnusedMediaItem(mediaItems []MediaItem, state ApplicationState) MediaItem {
	unusedItems := []MediaItem{}
	for _, item := range mediaItems {
		if !state.Contains(item) {
			unusedItems = append(unusedItems, item)
		}
	}

	idx := rand.Intn(len(unusedItems))
	return unusedItems[idx]
}
