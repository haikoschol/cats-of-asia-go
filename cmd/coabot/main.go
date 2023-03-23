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
	google_photos "github.com/haikoschol/cats-of-asia/internal/gphotos"
	"github.com/haikoschol/cats-of-asia/internal/state_json"
	"github.com/haikoschol/cats-of-asia/internal/twitter"
	_ "github.com/joho/godotenv/autoload"
	_ "image/jpeg"
	"log"
	"os"
)

var (
	albumId                 = os.Getenv("COABOT_ALBUM_ID")
	statePath               = os.Getenv("COABOT_STATE_FILE")
	oauthAppCredentialsPath = os.Getenv("COABOT_OAUTH_APP_CREDENTIALS_FILE")
	googlePhotosTokenPath   = os.Getenv("COABOT_GOOGLE_PHOTOS_TOKEN_FILE")
	twitterConsumerKey      = os.Getenv("COABOT_TWITTER_CONSUMER_KEY")
	twitterConsumerSecret   = os.Getenv("COABOT_TWITTER_CONSUMER_SECRET")
	twitterAccessToken      = os.Getenv("COABOT_TWITTER_ACCESS_TOKEN")
	twitterAccessSecret     = os.Getenv("COABOT_TWITTER_ACCESS_SECRET")
)

func main() {
	validateEnv()

	state, err := state_json.New(statePath)
	if err != nil {
		log.Fatal(err)
	}

	album, err := google_photos.New(albumId, oauthAppCredentialsPath, googlePhotosTokenPath)
	if err != nil {
		log.Fatal(err)
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

	mediaContent, err := album.GetContentFromMediaItem(mediaItem)
	if err != nil {
		log.Fatal(err)
	}

	// TODO add location and format date with local timezone
	description := fmt.Sprintf("Another fine feline, captured at %s", mediaItem.CreationTime)

	if err := publisher.Publish(mediaItem, mediaContent, description); err != nil {
		log.Fatal(err)
	}

	if err := state.Add(mediaItem); err != nil {
		log.Fatal(err)
	}
}

func validateEnv() {
	if albumId == "" {
		log.Fatal("COABOT_ALBUM_ID env var missing")
	}
	if statePath == "" {
		log.Fatal("COABOT_STATE_FILE env var missing")
	}
	if oauthAppCredentialsPath == "" {
		log.Fatal("COABOT_OAUTH_APP_CREDENTIALS_FILE env var missing")
	}
	if googlePhotosTokenPath == "" {
		log.Fatal("COABOT_GOOGLE_PHOTOS_TOKEN_FILE env var missing")
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
