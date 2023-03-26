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

package filesystem_album

import (
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	"github.com/rwcarlsen/goexif/exif"
	"os"
	"path"
	"strings"
	"time"
)

type filesystemAlbum struct {
	basePath string
}

func New(basePath string) (coabot.MediaAlbum, error) {
	if !path.IsAbs(basePath) {
		return nil, fmt.Errorf("base path needs to be absolute. this one isn't: %s", basePath)
	}

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read base path %s: %w", basePath, err)
	}

	hasMedia := false
	for _, entry := range entries {
		if !entry.IsDir() && isSupportedMedia(entry.Name()) {
			hasMedia = true
			break
		}
	}

	if !hasMedia {
		return nil, fmt.Errorf("directory %s contains no supported media files", basePath)
	}

	return filesystemAlbum{
		basePath,
	}, nil
}

func (fsa filesystemAlbum) Id() string {
	return fsa.basePath
}

func (fsa filesystemAlbum) GetMediaItems() ([]coabot.MediaItem, error) {
	entries, err := os.ReadDir(fsa.basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read base path %s: %w", fsa.basePath, err)
	}

	items := []coabot.MediaItem{}
	for _, entry := range entries {
		if isSupportedMedia(entry.Name()) {
			meta, err := fsa.getMetadata(entry.Name())
			if err != nil {
				return nil, err
			}

			item := coabot.MediaItem{
				Id:           entry.Name(),
				Filename:     entry.Name(),
				BaseUrl:      path.Join(fsa.basePath, entry.Name()),
				AlbumId:      fsa.Id(),
				CreationTime: meta.creationTime,
				Latitude:     meta.latitude,
				Longitude:    meta.longitude,
				Category:     coabot.Photo, // TODO support video
			}

			items = append(items, item)
		}
	}
	return items, nil
}

func (fsa filesystemAlbum) GetContentFromMediaItem(item coabot.MediaItem) (coabot.MediaContent, error) {
	mipath := path.Join(fsa.basePath, item.Filename)
	data, err := os.ReadFile(mipath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file at %s: %w", mipath, err)
	}
	return data, nil
}

type metadata struct {
	creationTime time.Time
	latitude     float64
	longitude    float64
}

func (fsa filesystemAlbum) getMetadata(filename string) (*metadata, error) {
	mipath := path.Join(fsa.basePath, filename)
	mediaFile, err := os.Open(mipath)
	if err != nil {
		return nil, fmt.Errorf("unable to read exif data from file at %s: %w", mipath, err)
	}
	defer mediaFile.Close()

	exifData, err := exif.Decode(mediaFile)
	if err != nil {
		return nil, fmt.Errorf("unable to decode exif data from file at %s: %w", mipath, err)
	}

	latitude, longitude, err := exifData.LatLong()
	if err != nil {
		return nil, fmt.Errorf("unable to read GPS coords from  exif data in file at %s: %w", mipath, err)
	}

	creationTime, err := exifData.DateTime()
	if err != nil {
		return nil, fmt.Errorf("unable to read timestamp from  exif data in file at %s: %w", mipath, err)
	}

	return &metadata{
		creationTime,
		latitude,
		longitude,
	}, nil
}

func isSupportedMedia(filename string) bool {
	filename = strings.ToLower(filename)
	return strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg")
}
