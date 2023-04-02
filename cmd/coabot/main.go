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

package main

import (
	coabot "github.com/haikoschol/cats-of-asia"
	"github.com/haikoschol/cats-of-asia/internal/bot"
	filesystem_album "github.com/haikoschol/cats-of-asia/internal/fsalbum"
	google_maps_geocoder "github.com/haikoschol/cats-of-asia/internal/gmaps"
	google_photos "github.com/haikoschol/cats-of-asia/internal/gphotos"
	"github.com/haikoschol/cats-of-asia/internal/mastodon"
	"github.com/haikoschol/cats-of-asia/internal/state_json"
	"github.com/haikoschol/cats-of-asia/internal/twitter"
	_ "github.com/joho/godotenv/autoload"
	_ "image/jpeg"
	"log"
	"os"
)

var (
	statePath = os.Getenv("COABOT_STATE_FILE")

	// Either COABOT_ALBUM_BASE_PATH or COABOT_GOOGLE_PHOTOS_* must be set. If both are, a filesystem-backed album will
	// be used.
	fsAlbumBasePath = os.Getenv("COABOT_ALBUM_BASE_PATH")

	googlePhotosAlbumId         = os.Getenv("COABOT_GOOGLE_PHOTOS_ALBUM_ID")
	googlePhotosCredentialsPath = os.Getenv("COABOT_GOOGLE_PHOTOS_CREDENTIALS_FILE")
	googlePhotosTokenPath       = os.Getenv("COABOT_GOOGLE_PHOTOS_TOKEN_FILE")

	googleMapsApiKey = os.Getenv("COABOT_GOOGLE_MAPS_API_KEY")

	mastodonServer      = os.Getenv("COABOT_MASTODON_SERVER")
	mastodonAccessToken = os.Getenv("COABOT_MASTODON_ACCESS_TOKEN")

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

	var geocoder coabot.Geocoder
	if googleMapsApiKey != "" {
		geocoder, err = google_maps_geocoder.New(googleMapsApiKey)
		if err != nil {
			log.Fatal(err)
		}
	}

	publishers, err := buildPublishers()
	if err != nil {
		log.Fatal(err)
	}

	bobTheBot, err := bot.New(state, album, publishers[0], geocoder, 4242)
	if err != nil {
		log.Fatal(err)
	}

	if len(publishers) > 1 {
		for _, publisher := range publishers {
			bobTheBot.AddPublisher(publisher)
		}
	}

	if err := bobTheBot.GoOutIntoTheWorldAndDoBotThings(); err != nil {
		log.Fatal(err)
	}
}

func buildPublishers() ([]coabot.Publisher, error) {
	publishers := []coabot.Publisher{}

	// should be unneccesary to check all mastodon config vars since validateEnv() already did that
	if mastodonServer != "" {
		mp, err := mastodon.New(mastodonServer, mastodonAccessToken)
		if err != nil {
			return nil, err
		}
		publishers = append(publishers, mp)
	}

	if twitterConsumerKey != "" {
		tp := twitter.New(twitter.Credentials{
			ConsumerKey:    twitterConsumerKey,
			ConsumerSecret: twitterConsumerSecret,
			AccessToken:    twitterAccessToken,
			AccessSecret:   twitterAccessSecret,
		})
		publishers = append(publishers, tp)
	}

	return publishers, nil
}

// the logic is getting very hairy with all this optional config. should probably use a robust env/config mgmt library
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

	if twitterConsumerKey == "" && twitterConsumerSecret == "" && twitterAccessToken == "" && twitterAccessSecret == "" {
		if mastodonServer == "" && mastodonAccessToken == "" {
			log.Fatal("either COABOT_MASTODON_* or COABOT_TWITTER_* env vars need to be set")
		}
		if mastodonServer == "" {
			log.Fatal("COABOT_MASTODON_SERVER env var missing")
		}
		if mastodonAccessToken == "" {
			log.Fatal("COABOT_MASTODON_ACCESS_TOKEN env var missing")
		}
	} else {
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
}
