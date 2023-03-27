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

package main

import (
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	filesystem_album "github.com/haikoschol/cats-of-asia/internal/fsalbum"
	google_photos "github.com/haikoschol/cats-of-asia/internal/gphotos"
	"github.com/haikoschol/cats-of-asia/internal/state_json"
	"github.com/haikoschol/cats-of-asia/internal/twitter"
	_ "github.com/joho/godotenv/autoload"
	_ "image/jpeg"
	"log"
	"os"
)

var (
	statePath = os.Getenv("COABOT_STATE_FILE")

	// Either COABOT_ALBUM_BASE_PATH is set or COABOT_GOOGLE_PHOTOS_* are, of both are set, a filesystem-backed album
	// will be used.
	fsAlbumBasePath = os.Getenv("COABOT_ALBUM_BASE_PATH")

	googlePhotosAlbumId         = os.Getenv("COABOT_GOOGLE_PHOTOS_ALBUM_ID")
	googlePhotosCredentialsPath = os.Getenv("COABOT_GOOGLE_PHOTOS_CREDENTIALS_FILE")
	googlePhotosTokenPath       = os.Getenv("COABOT_GOOGLE_PHOTOS_TOKEN_FILE")

	twitterConsumerKey    = os.Getenv("COABOT_TWITTER_CONSUMER_KEY")
	twitterConsumerSecret = os.Getenv("COABOT_TWITTER_CONSUMER_SECRET")
	twitterAccessToken    = os.Getenv("COABOT_TWITTER_ACCESS_TOKEN")
	twitterAccessSecret   = os.Getenv("COABOT_TWITTER_ACCESS_SECRET")
)

func main() {
	validateEnv()

	state, err := state_json.New(statePath)
	if err != nil {
		log.Fatal(err)
	}

	var album coabot.MediaAlbum

	if fsAlbumBasePath != "" {
		album, err = filesystem_album.New(fsAlbumBasePath)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		album, err = google_photos.New(googlePhotosAlbumId, googlePhotosCredentialsPath, googlePhotosTokenPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	publisher := twitter.New(twitter.Credentials{
		ConsumerKey:    twitterConsumerKey,
		ConsumerSecret: twitterConsumerSecret,
		AccessToken:    twitterAccessToken,
		AccessSecret:   twitterAccessSecret,
	})

	mediaItems, err := album.GetMediaItems()
	if err != nil {
		log.Fatal(err)
	}

	mediaItem := coabot.PickRandomUnusedMediaItem(mediaItems, state)

	meta, err := mediaItem.Metadata()
	if err != nil {
		log.Fatal(err)
	}

	// TODO replace lat/long with name of nearest city
	description := fmt.Sprintf(
		"Another fine feline, captured at time %s and location %f, %f",
		meta.CreationTime,
		meta.Latitude,
		meta.Longitude,
	)

	if err := publisher.Publish(mediaItem, description); err != nil {
		log.Fatal(err)
	}

	if err := state.Add(mediaItem); err != nil {
		log.Fatal(err)
	}
}

func validateEnv() {
	if fsAlbumBasePath == "" {
		if googlePhotosAlbumId == "" || googlePhotosCredentialsPath == "" || googlePhotosTokenPath == "" {
			log.Fatal("either COABOT_ALBUM_BASE_PATH or COABOT_GOOGLE_PHOTOS_ALBUM_ID, " +
				"COABOT_GOOGLE_PHOTOS_CREDENTIALS_FILE and COABOT_GOOGLE_PHOTOS_TOKEN_FILE need to be set")
		}

		bail := false
		if googlePhotosAlbumId == "" {
			log.Print("COABOT_GOOGLE_PHOTOS_ALBUM_ID env var missing")
			bail = true
		}
		if googlePhotosCredentialsPath == "" {
			log.Print("COABOT_GOOGLE_PHOTOS_CREDENTIALS_FILE env var missing")
			bail = true
		}
		if googlePhotosTokenPath == "" {
			log.Print("COABOT_GOOGLE_PHOTOS_TOKEN_FILE env var missing")
			bail = true
		}
		if bail {
			os.Exit(1)
		}
	}

	if statePath == "" {
		log.Fatal("COABOT_STATE_FILE env var missing")
	}
	if twitterConsumerKey == "" {
		log.Fatal("COABOT_TWITTER_CONSUMER_KEY env var missing")
	}
	if twitterConsumerSecret == "" {
		log.Fatal("COABOT_TWITTER_CONSUMER_SECRET env var missing")
	}
	if twitterAccessToken == "" {
		log.Fatal("COABOT_TWITTER_ACCESS_TOKEN env var missing")
	}
	if twitterAccessSecret == "" {
		log.Fatal("COABOT_TWITTER_ACCESS_SECRET env var missing")
	}
}
