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

package filesystem_album

import (
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	"github.com/rwcarlsen/goexif/exif"
	"io"
	"os"
	"path"
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
		if !entry.IsDir() && coabot.IsSupportedMedia(entry.Name()) {
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
		if coabot.IsSupportedMedia(entry.Name()) {
			item := fsMediaItem{
				filename: entry.Name(),
				basePath: fsa.basePath,
				albumId:  fsa.Id(),
			}

			items = append(items, item)
		}
	}
	return items, nil
}

type fsMediaItem struct {
	filename string
	basePath string
	albumId  string
}

func (fsi fsMediaItem) Id() string {
	return path.Join(fsi.albumId, fsi.filename)
}

func (fsi fsMediaItem) Filename() string {
	return fsi.filename
}

func (fsi fsMediaItem) Category() coabot.MediaCategory {
	return coabot.Photo // TODO support video
}

func (fsi fsMediaItem) Metadata() (*coabot.MediaMetadata, error) {
	mipath := path.Join(fsi.basePath, fsi.filename)
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

	return &coabot.MediaMetadata{
		CreationTime: creationTime,
		Latitude:     latitude,
		Longitude:    longitude,
	}, nil
}

func (fsi fsMediaItem) Content() ([]byte, error) {
	mipath := path.Join(fsi.basePath, fsi.filename)
	data, err := os.ReadFile(mipath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file at %s: %w", mipath, err)
	}
	return data, nil
}

func (fsi fsMediaItem) Read() (io.ReadCloser, error) {
	mipath := path.Join(fsi.basePath, fsi.filename)
	f, err := os.Open(mipath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file at %s: %w", mipath, err)
	}
	return f, err
}
