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
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/haikoschol/cats-of-asia/internal/bot"
	"github.com/haikoschol/cats-of-asia/internal/mastodon"
	"github.com/haikoschol/cats-of-asia/internal/twitter"
	"github.com/haikoschol/cats-of-asia/pkg/postgres"
	_ "github.com/joho/godotenv/autoload"
	"github.com/matrix-org/gomatrix"
	_ "image/jpeg"
	"log"
	"os"
)

var (
	dbHost     = os.Getenv("COA_DB_HOST")
	dbSSLMode  = os.Getenv("COA_DB_SSLMODE")
	dbName     = os.Getenv("COA_DB_NAME")
	dbUser     = os.Getenv("COA_DB_USER")
	dbPassword = os.Getenv("COA_DB_PASSWORD")

	mastodonServer      = os.Getenv("COABOT_MASTODON_SERVER")
	mastodonAccessToken = os.Getenv("COABOT_MASTODON_ACCESS_TOKEN")

	twitterConsumerKey    = os.Getenv("COABOT_TWITTER_CONSUMER_KEY")
	twitterConsumerSecret = os.Getenv("COABOT_TWITTER_CONSUMER_SECRET")
	twitterAccessToken    = os.Getenv("COABOT_TWITTER_ACCESS_TOKEN")
	twitterAccessSecret   = os.Getenv("COABOT_TWITTER_ACCESS_SECRET")

	matrixServer      = os.Getenv("COABOT_MATRIX_SERVER")
	matrixUser        = os.Getenv("COABOT_MATRIX_USER")
	matrixAccessToken = os.Getenv("COABOT_MATRIX_ACCESS_TOKEN")
	matrixLogRoomId   = os.Getenv("COABOT_MATRIX_LOG_ROOM_ID")
)

func main() {
	validateEnv()

	db, err := postgres.NewDatabase(dbUser, dbPassword, dbHost, dbName, postgres.SSLMode(dbSSLMode))
	if err != nil {
		log.Fatal(err)
	}

	publishers, err := buildPublishers()
	if err != nil {
		log.Fatal(err)
	}

	matrix, err := gomatrix.NewClient(matrixServer, matrixUser, matrixAccessToken)
	if err != nil {
		log.Fatal(err)
	}

	bobTheBot, err := bot.NewBot(db, publishers[0], matrix, matrixLogRoomId, 4242)
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

func buildPublishers() ([]coa.Publisher, error) {
	publishers := []coa.Publisher{}

	// should be unneccesary to check all mastodon config vars since validateEnv() already did that
	if mastodonServer != "" {
		mp, err := mastodon.New(mastodonServer, mastodonAccessToken, []string{"#CatsOfAsia", "#CatsOfMastodon"})
		if err != nil {
			return nil, err
		}
		publishers = append(publishers, mp)
	}

	if twitterConsumerKey != "" {
		tp := twitter.NewPublisher(twitter.Credentials{
			ConsumerKey:    twitterConsumerKey,
			ConsumerSecret: twitterConsumerSecret,
			AccessToken:    twitterAccessToken,
			AccessSecret:   twitterAccessSecret,
		})
		publishers = append(publishers, tp)
	}

	return publishers, nil
}

// having these funcs in all executables is ugly. should probably use a robust env/config mgmt library
func validateEnv() {
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

	bail := false
	if matrixServer == "" {
		log.Print("COABOT_MATRIX_SERVER env var missing")
		bail = true
	}
	if matrixUser == "" {
		log.Print("COABOT_MATRIX_USER env var missing")
		bail = true
	}
	if matrixAccessToken == "" {
		log.Print("COABOT_MATRIX_ACCESS_TOKEN env var missing")
		bail = true
	}
	if matrixLogRoomId == "" {
		log.Print("COABOT_MATRIX_LOG_ROOM_ID env var missing")
		bail = true
	}
	if bail {
		os.Exit(1)
	}
}
