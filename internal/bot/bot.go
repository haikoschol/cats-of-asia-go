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

package bot

import (
	"errors"
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	"log"
	"net/http"
	"strings"
)

const maxGeocodingTries = 20

type Bot struct {
	state      coabot.ApplicationState
	album      coabot.MediaAlbum
	publishers []coabot.Publisher
	geocoder   coabot.Geocoder
	listenPort int
}

func New(
	state coabot.ApplicationState,
	album coabot.MediaAlbum,
	publisher coabot.Publisher,
	geocoder coabot.Geocoder,
	listenPort int,
) (*Bot, error) {
	if state == nil {
		return nil, errors.New("state is nil")
	}
	if album == nil {
		return nil, errors.New("album is nil")
	}
	if publisher == nil {
		return nil, errors.New("publisher is nil")
	}
	if geocoder == nil {
		return nil, errors.New("geocoder is nil")
	}

	return &Bot{
		state:      state,
		album:      album,
		publishers: []coabot.Publisher{publisher},
		geocoder:   geocoder,
		listenPort: listenPort,
	}, nil
}

func (b *Bot) AddPublisher(p coabot.Publisher) {
	b.publishers = append(b.publishers, p)
}

func (b *Bot) GoOutIntoTheWorldAndDoBotThings() error {
	http.HandleFunc("/", b.post)
	return http.ListenAndServe(fmt.Sprintf(":%d", b.listenPort), nil)
}

func (b *Bot) post(w http.ResponseWriter, req *http.Request) {
	if !validateRequest(w, req) {
		return
	}

	item, err := b.pickMediaItem()
	if err != nil {
		b.handleError(err, w)
		return
	}

	published := false
	for _, pub := range b.publishers {
		if err := pub.Publish(item.mediaItem, item.description); err != nil {
			log.Printf(
				"unable to publish file %s from album %s to %s: %v\n",
				item.mediaItem.Filename(),
				b.album.Id(),
				pub.Name(),
				err,
			)
		} else {
			published = true
		}
	}

	if published {
		if err := b.state.Add(item.mediaItem); err != nil {
			log.Print(err)
			return
		}
	} else {
		err := errors.New("failed to publish media to any platform")
		b.handleError(err, w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type itemWithDescription struct {
	mediaItem   coabot.MediaItem
	description string
}

func (b *Bot) pickMediaItem() (*itemWithDescription, error) {
	mediaItems, err := b.album.GetMediaItems()
	if err != nil {
		return nil, err
	}

	var item *itemWithDescription
	tries := 0
	for item == nil && tries < maxGeocodingTries {
		mediaItem := coabot.PickRandomUnusedMediaItem(mediaItems, b.state)

		meta, err := mediaItem.Metadata()
		if err != nil {
			return nil, err
		}

		location, err := b.geocoder.LookupCityAndCountry(meta.Latitude, meta.Longitude)
		if err != nil {
			log.Printf(
				"reverse geocoding failed for file %s in album %s: %v\n",
				mediaItem.Filename(),
				b.album.Id(),
				err,
			)
			mediaItems = b.removeMediaItem(mediaItem, mediaItems)
			tries += 1
			continue
		}

		description, err := b.buildDescription(meta, location)
		if err != nil {
			return nil, err
		}

		item = &itemWithDescription{
			mediaItem,
			description,
		}
	}

	return item, nil
}

func (b *Bot) buildDescription(meta *coabot.MediaMetadata, location coabot.CityAndCountry) (string, error) {
	description := fmt.Sprintf(
		"Another fine feline, captured in %v on %v, %v %d %d",
		location,
		meta.CreationTime.Weekday(),
		meta.CreationTime.Month(),
		meta.CreationTime.Day(),
		meta.CreationTime.Year(),
	)

	return description, nil
}

func (b *Bot) handleError(err error, w http.ResponseWriter) {
	log.Print(err)
	w.WriteHeader(http.StatusInternalServerError)
}

func (b *Bot) removeMediaItem(item coabot.MediaItem, items []coabot.MediaItem) []coabot.MediaItem {
	res := []coabot.MediaItem{}
	for _, mi := range items {
		if mi.Id() != item.Id() {
			res = append(res, mi)
		}
	}
	return res
}

func validateRequest(w http.ResponseWriter, req *http.Request) bool {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}

	if !strings.HasPrefix(req.RemoteAddr, "127.0.0.1") {
		w.WriteHeader(http.StatusForbidden)
		return false
	}

	return true
}
