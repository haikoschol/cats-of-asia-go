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
	"github.com/matrix-org/gomatrix"
	"log"
	"net/http"
	"strings"
)

const maxGeocodingTries = 20

const matrixHelpText = `available commands are:
- help: you are looking at it
- albumId: responds with the ID of the album which is the source of cat content to post on Mastodon/Twitter
- files: lists the names of all files in the album
- metadata <filename>: retrieves the metadata of a given file from the album
- geocode <filename>: reverse geocode the lat/long coordinates in the given file
- unusedCount: responds with the number of not yet posted files in the album
`

type Bot struct {
	state      coabot.ApplicationState
	album      coabot.MediaAlbum
	publishers []coabot.Publisher
	geocoder   coabot.Geocoder
	listenPort int
	matrix     *gomatrix.Client
	logRoomId  string
}

func New(
	state coabot.ApplicationState,
	album coabot.MediaAlbum,
	publisher coabot.Publisher,
	geocoder coabot.Geocoder,
	matrix *gomatrix.Client,
	logRoomId string,
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
	if matrix == nil {
		return nil, errors.New("matrix is nil")
	}

	return &Bot{
		state:      state,
		album:      album,
		publishers: []coabot.Publisher{publisher},
		geocoder:   geocoder,
		listenPort: listenPort,
		matrix:     matrix,
		logRoomId:  logRoomId,
	}, nil
}

func (b *Bot) AddPublisher(p coabot.Publisher) {
	b.publishers = append(b.publishers, p)
}

func (b *Bot) GoOutIntoTheWorldAndDoBotThings() error {
	syncer := b.matrix.Syncer.(*gomatrix.DefaultSyncer)
	syncer.OnEventType("m.room.message", b.handleMatrixMessage)

	// TODO teardown
	go func() {
		for {
			if err := b.matrix.Sync(); err != nil {
				log.Printf("unable to sync state with matrix server %v: %v\n", b.matrix.HomeserverURL, err)
			}
		}
	}()

	http.HandleFunc("/", b.post)
	return http.ListenAndServe(fmt.Sprintf(":%d", b.listenPort), nil)
}

// post contains the "business logic" of the bot
func (b *Bot) post(w http.ResponseWriter, req *http.Request) {
	if !validateRequest(w, req) {
		b.log(fmt.Sprintf("ignoring invalid request from '%s'", req.RemoteAddr))
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
			b.logError(fmt.Errorf(
				"unable to publish file '%s' from album '%s' to %s: %w",
				item.mediaItem.Filename(),
				b.album.Id(),
				pub.Name(),
				err,
			))
		} else {
			published = true
		}
	}

	if !published {
		err := errors.New("failed to publish media to any platform")
		b.handleError(err, w)
		return
	}

	if err := b.state.Add(item.mediaItem); err != nil {
		b.logError(err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *Bot) handleMatrixMessage(ev *gomatrix.Event) {
	body, ok := ev.Body()
	if !ok {
		return
	}

	suffix := fmt.Sprintf(":%s", b.matrix.HomeserverURL.Host)
	shortUid, _ := strings.CutSuffix(b.matrix.UserID, suffix)
	body, found := strings.CutPrefix(body, shortUid)
	if !found {
		return
	}

	body = strings.TrimSpace(body)
	cmd, args, _ := strings.Cut(body, " ")
	cmd = strings.TrimSpace(cmd)
	args = strings.TrimSpace(args)

	b.handleMatrixCommand(ev, cmd, args)
}

func (b *Bot) handleMatrixCommand(ev *gomatrix.Event, command, arguments string) {
	switch command {
	case "help":
		b.sendCommandResponse(ev, matrixHelpText)
	case "albumId":
		b.sendCommandResponse(ev, b.album.Id())
	case "files":
		b.handleFilesCommand(ev)
	case "metadata":
		b.handleMetadataCommand(ev, arguments)
	case "geocode":
		b.handleGeocodeCommand(ev, arguments)
	case "unusedCount":
		b.handleUnusedCountCommand(ev)
	default:
		message := fmt.Sprintf("unknown command '%s'. Use 'help' to list all available commands", command)
		b.sendCommandResponse(ev, message)
	}
}

func (b *Bot) handleFilesCommand(ev *gomatrix.Event) {
	mediaItems, err := b.album.GetMediaItems()
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve media items from album '%s': %v", b.album.Id(), err))
		return
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("files in album '%s':\n", b.album.Id()))

	for _, item := range mediaItems {
		builder.WriteString(fmt.Sprintf("%s\n", item.Filename()))
	}

	b.sendCommandResponse(ev, builder.String())
}

func (b *Bot) handleMetadataCommand(ev *gomatrix.Event, filename string) {
	filename = strings.TrimSpace(filename)

	mediaItems, err := b.album.GetMediaItems()
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve media items from album '%s': %v", b.album.Id(), err))
		return
	}

	var mediaItem coabot.MediaItem
	for _, item := range mediaItems {
		if filename == item.Filename() {
			mediaItem = item
			break
		}
	}

	if mediaItem == nil {
		b.sendCommandResponse(ev, fmt.Sprintf("no file named '%s' found in album '%s'", filename, b.album.Id()))
		return
	}

	metadata, err := mediaItem.Metadata()
	if err != nil {
		b.logResponse(ev, err.Error())
		return
	}

	message := fmt.Sprintf(`metadata for file '%s' in album '%s':
DateTimeOriginal: %s
Latitude: %f
Longitude: %f
`,
		mediaItem.Filename(),
		b.album.Id(),
		metadata.CreationTime,
		metadata.Latitude,
		metadata.Longitude,
	)

	b.sendCommandResponse(ev, message)
}

// TODO DRY
func (b *Bot) handleGeocodeCommand(ev *gomatrix.Event, filename string) {
	filename = strings.TrimSpace(filename)

	mediaItems, err := b.album.GetMediaItems()
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve media items from album '%s': %v", b.album.Id(), err))
		return
	}

	var mediaItem coabot.MediaItem
	for _, item := range mediaItems {
		if filename == item.Filename() {
			mediaItem = item
			break
		}
	}

	if mediaItem == nil {
		b.sendCommandResponse(ev, fmt.Sprintf("no file named '%s' found in album '%s'", filename, b.album.Id()))
		return
	}

	metadata, err := mediaItem.Metadata()
	if err != nil {
		b.logResponse(ev, err.Error())
		return
	}

	location, err := b.geocoder.LookupCityAndCountry(metadata.Latitude, metadata.Longitude)
	if err != nil {
		b.logResponse(ev, err.Error())
	}

	message := fmt.Sprintf(
		"media file '%s' in album '%s' was created in %s",
		mediaItem.Filename(),
		b.album.Id(),
		location.String(),
	)

	b.sendCommandResponse(ev, message)
}

func (b *Bot) handleUnusedCountCommand(ev *gomatrix.Event) {
	mediaItems, err := b.album.GetMediaItems()
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve media items from album '%s': %v", b.album.Id(), err))
		return
	}

	count := 0
	for _, item := range mediaItems {
		if !b.state.Contains(item) {
			count += 1
		}
	}

	b.sendCommandResponse(ev, fmt.Sprintf("album '%s' contains %d unused media files", b.album.Id(), count))
}

// sendCommandResponse sends the message to the sender of the command and only logs locally in case of error
func (b *Bot) sendCommandResponse(ev *gomatrix.Event, message string) {
	message = fmt.Sprintf("%s %s", ev.Sender, message)

	_, err := b.matrix.SendText(ev.RoomID, message)
	if err != nil {
		log.Printf(
			"unable to send command response to matrix server %v. error: '%v' message: '%s'\n",
			b.matrix.HomeserverURL,
			err,
			message,
		)
	}
}

// logResponse sends the message to the sender of the command and logs locally in any case
func (b *Bot) logResponse(ev *gomatrix.Event, message string) {
	message = fmt.Sprintf("%s %s", ev.Sender, message)

	if _, err := b.matrix.SendText(ev.RoomID, message); err != nil {
		log.Printf(
			"unable to send log response to room '%s' on matrix server %v. error: '%v' message: '%s'\n",
			ev.RoomID,
			b.matrix.HomeserverURL,
			err,
			message,
		)
	} else {
		log.Printf(
			"sent log response to room '%s' on matrix server %v. message: '%s'\n",
			ev.RoomID,
			b.matrix.HomeserverURL,
			message,
		)
	}
}

func (b *Bot) log(message string) {
	if _, err := b.matrix.SendText(b.logRoomId, message); err != nil {
		log.Printf(
			"unable to send below log message to room '%s' on matrix server %v. error: '%v'\n",
			b.logRoomId,
			b.matrix.HomeserverURL,
			err,
		)
	}
	log.Println(message)
}

// logError sends the string representation of the given error to the default logging room on matrix and logs it locally
func (b *Bot) logError(lErr error) {
	if _, err := b.matrix.SendText(b.logRoomId, lErr.Error()); err != nil {
		log.Printf(
			"unable to send error log message to room '%s' on matrix server %v. error to log: '%v' error while sending: '%v'\n",
			b.logRoomId,
			b.matrix.HomeserverURL,
			lErr,
			err,
		)
	} else {
		log.Printf(
			"sent error log message to room '%s' on matrix server %v. logged error: '%v'\n",
			b.logRoomId,
			b.matrix.HomeserverURL,
			err,
		)
	}
}

func (b *Bot) handleError(err error, w http.ResponseWriter) {
	b.logError(err)
	w.WriteHeader(http.StatusInternalServerError)
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
			b.logError(fmt.Errorf(
				"reverse geocoding failed for file '%s' in album '%s': '%v'. picking another one...\n",
				mediaItem.Filename(),
				b.album.Id(),
				err,
			))
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

func (b *Bot) removeMediaItem(item coabot.MediaItem, items []coabot.MediaItem) []coabot.MediaItem {
	res := []coabot.MediaItem{}
	for _, mi := range items {
		if mi.Id() != item.Id() {
			res = append(res, mi)
		}
	}
	return res
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
