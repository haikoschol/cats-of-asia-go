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
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/matrix-org/gomatrix"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//const maxGeocodingTries = 20

const matrixHelpText = `available commands are:
- help: you are looking at it
- images: list the IDs and paths of all images in the database
- metadata <imageID>: show the metadata of a image
- unusedCount: list the number of not yet posted images for each supported platform
`

type Bot struct {
	db         coa.Database
	publishers []coa.Publisher
	listenPort int
	matrix     *gomatrix.Client
	logRoomId  string
}

func NewBot(
	db coa.Database,
	publisher coa.Publisher,
	matrix *gomatrix.Client,
	logRoomId string,
	listenPort int,
) (*Bot, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if publisher == nil {
		return nil, errors.New("publisher is nil")
	}
	if matrix == nil {
		return nil, errors.New("matrix is nil")
	}

	return &Bot{
		db:         db,
		publishers: []coa.Publisher{publisher},
		listenPort: listenPort,
		matrix:     matrix,
		logRoomId:  logRoomId,
	}, nil
}

func (b *Bot) AddPublisher(p coa.Publisher) {
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

	published := false
	for _, pub := range b.publishers {
		img, err := b.db.GetRandomUnusedImage(pub.Platform())
		if err != nil {
			err = fmt.Errorf("failed to fetch random unused image for platform '%s' from db: %w", pub.Platform(), err)
			b.logError(err)
			continue
		}

		if err := pub.Publish(img, b.buildDescription(img)); err != nil {
			b.logError(fmt.Errorf(
				"failed to publish file '%s' on platform %s: %w",
				img.PathLarge,
				pub.Platform(),
				err,
			))
		} else {
			err := b.db.InsertPost(img, pub.Platform())
			if err != nil {
				b.logError(fmt.Errorf(
					"failed to insert post of file '%s' on platform %s: %w",
					img.PathLarge,
					pub.Platform(),
					err,
				))
			}
			// set this to true regardless of InsertPost() failing since the image was actually posted successfully
			published = true
		}
	}

	if !published {
		err := errors.New("failed to publish media to any platform")
		b.handleError(err, w)
		return
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

func (b *Bot) handleMatrixCommand(ev *gomatrix.Event, command, args string) {
	switch command {
	case "help":
		b.sendCommandResponse(ev, matrixHelpText)
	case "images":
		b.handleImagesCommand(ev)
	case "metadata":
		b.handleMetadataCommand(ev, args)
	case "unusedCount":
		b.handleUnusedCountCommand(ev)
	default:
		message := fmt.Sprintf("unknown command '%s'. Use 'help' to list all available commands", command)
		b.sendCommandResponse(ev, message)
	}
}

func (b *Bot) handleImagesCommand(ev *gomatrix.Event) {
	images, err := b.db.GetImages()
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve list of files from db: %v", err))
		return
	}

	// TODO format as HTML table
	var builder strings.Builder
	for _, img := range images {
		builder.WriteString(fmt.Sprintf("%d\t%s\n", img.ID, img.PathLarge))
	}

	b.sendCommandResponse(ev, builder.String())
}

func (b *Bot) handleMetadataCommand(ev *gomatrix.Event, args string) {
	arg, _, _ := strings.Cut(args, " ")
	imgID, err := strconv.Atoi(arg)
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("invalid image ID: '%s'", arg))
		return
	}

	img, err := b.db.GetImage(int64(imgID))
	if err != nil {
		b.logResponse(ev, fmt.Sprintf("unable to retrieve image metadata for image %d from db: %v", imgID, err))
		return
	}

	// TODO format as HTML table
	message := fmt.Sprintf(`metadata for image %d:
PathLarge: %s
PathMedium: %s
PathSmall: %s
SHA256: %s
Timestamp: %s
Timezone: %s
Latitude: %f
Longitude: %f
City: %s
Country: %s
`,
		img.ID,
		img.PathLarge,
		img.PathMedium,
		img.PathSmall,
		img.SHA256,
		img.Timestamp.Format(time.DateTime),
		img.Timezone,
		img.Latitude,
		img.Longitude,
		img.City,
		img.Country,
	)

	b.sendCommandResponse(ev, message)
}

func (b *Bot) handleUnusedCountCommand(ev *gomatrix.Event) {
	var builder strings.Builder

	mastodon, err := b.db.GetUnusedImageCount(coa.Mastodon)
	if err != nil {
		resp := fmt.Sprintf("unable to retrieve unused image count for platform %s from db: %v", coa.Mastodon, err)
		b.logResponse(ev, resp)
	}

	// TODO format as HTML table
	builder.WriteString(fmt.Sprintf("%s: %d\n", coa.Mastodon, mastodon))

	x, err := b.db.GetUnusedImageCount(coa.X)
	if err != nil {
		resp := fmt.Sprintf("unable to retrieve unused image count for platform %s from db: %v", coa.X, err)
		b.logResponse(ev, resp)
	}

	builder.WriteString(fmt.Sprintf("%s: %d\n", coa.X, x))
	b.sendCommandResponse(ev, builder.String())
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

func (b *Bot) buildDescription(img coa.Image) string {
	return fmt.Sprintf(
		"Another fine feline, captured in %v on %v, %v %d %d",
		img.Location(),
		img.Timestamp.Weekday(),
		img.Timestamp.Month(),
		img.Timestamp.Day(),
		img.Timestamp.Year(),
	)
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
