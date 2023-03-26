// Copyright (C) 2023 Haiko Schol
// SPDX-License-Identifier: GPL-3.0-or-later WITH Classpath-exception-2.0

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
	"math/rand"
	"time"
)

// MediaCategory denotes whether a media item is a photo or video.
type MediaCategory string

// MediaContent denotes the raw data of a photo or video.
type MediaContent []byte

const (
	Photo MediaCategory = "photo"
	Video               = "video"
)

// MediaItem uniquely identifies a photo or video in a MediaAlbum and contains all relevant metadata.
type MediaItem struct {
	// Id is an identifier specific to the MediaAlbum implementation this MediaItem belongs to. It is only guaranteed
	// to be unique in that album.
	Id string

	// AlbumId is the identifier of the MediaAlbum this MediaItem belongs to.
	AlbumId string

	// Filename of the photo or video.
	Filename string

	// CreationTime is the time when the photo or video was taken, in the timezone where it was taken.
	CreationTime time.Time // TODO change to method and lazy load in filesystem impl

	Latitude  float64 // TODO change to method and lazy load in filesystem impl
	Longitude float64 // TODO change to method and lazy load in filesystem impl
	BaseUrl   string  // FIXME this is a Google Photos implementation detail

	// Category denotes the type of media.
	Category MediaCategory
}

// MediaAlbum is a repository of media items.
type MediaAlbum interface {
	// Id returns an opaque identifier for the MediaAlbum
	Id() string

	// GetMediaItems lists all media items in the album.
	GetMediaItems() ([]MediaItem, error)

	// GetContentFromMediaItem retrieves the raw data of a photo or video represented by the given MediaItem.
	GetContentFromMediaItem(item MediaItem) (MediaContent, error)
}

// Publisher allows publishing photos or videos.
type Publisher interface {
	// Publish sends a photo or video together with a description to a platform.
	Publish(item MediaItem, content MediaContent, description string) error
}

// ApplicationState provides a persistence mechanism for keeping track of which items in a MediaAlbum have already been
// published.
type ApplicationState interface {
	// Add adds a MediaItem to the persistent application state.
	Add(item MediaItem) error

	// Contains checks whether the given MediaItem has already been added to the persistent application state.
	Contains(item MediaItem) bool
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
