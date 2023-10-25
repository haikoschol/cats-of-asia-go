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
	"errors"
	"fmt"
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/haikoschol/cats-of-asia/internal/mastodon"
	"github.com/haikoschol/cats-of-asia/internal/twitter"
	"github.com/haikoschol/cats-of-asia/pkg/postgres"
	"github.com/haikoschol/cats-of-asia/pkg/validation"
	_ "github.com/joho/godotenv/autoload"
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

	if err := publish(publishers, db); err != nil {
		log.Fatal(err)
	}
}

func publish(publishers []coa.Publisher, db coa.Database) error {
	published := false
	for _, pub := range publishers {
		img, err := db.GetRandomUnusedImage(pub.Platform())
		if err != nil {
			return fmt.Errorf("failed to fetch random unused image for platform '%s' from db: %w", pub.Platform(), err)
		}

		if err := pub.Publish(img, buildDescription(img)); err != nil {
			return fmt.Errorf(
				"failed to publish file '%s' on platform %s: %w",
				img.PathLarge,
				pub.Platform(),
				err,
			)
		} else {
			err := db.InsertPost(img, pub.Platform())
			if err != nil {
				return fmt.Errorf(
					"failed to insert post of file '%s' on platform %s: %w",
					img.PathLarge,
					pub.Platform(),
					err,
				)
			}
			// set this to true regardless of InsertPost() failing since the image was actually posted successfully
			published = true
		}
	}

	if !published {
		return errors.New("failed to publish media to any platform")
	}

	return nil
}

func buildDescription(img coa.Image) string {
	return fmt.Sprintf(
		"Another fine feline, captured in %v on %v, %v %d %d",
		img.Location(),
		img.Timestamp.Weekday(),
		img.Timestamp.Month(),
		img.Timestamp.Day(),
		img.Timestamp.Year(),
	)
}

func buildPublishers() ([]coa.Publisher, error) {
	var publishers []coa.Publisher

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

func validateEnv() {
	errs := validation.ValidateDbEnv(dbHost, dbSSLMode, dbName, dbUser, dbPassword)

	if twitterConsumerKey == "" && twitterConsumerSecret == "" && twitterAccessToken == "" && twitterAccessSecret == "" {
		if mastodonServer == "" && mastodonAccessToken == "" {
			errs = append(errs, "either COABOT_MASTODON_* or COABOT_TWITTER_* env vars need to be set")
		}
		if mastodonServer == "" {
			errs = append(errs, "COABOT_MASTODON_SERVER env var missing")
		}
		if mastodonAccessToken == "" {
			errs = append(errs, "COABOT_MASTODON_ACCESS_TOKEN env var missing")
		}
	} else {
		if twitterConsumerKey == "" {
			errs = append(errs, "COABOT_TWITTER_CONSUMER_KEY env var missing")
		}
		if twitterConsumerSecret == "" {
			errs = append(errs, "COABOT_TWITTER_CONSUMER_SECRET env var missing")
		}
		if twitterAccessToken == "" {
			errs = append(errs, "COABOT_TWITTER_ACCESS_TOKEN env var missing")
		}
		if twitterAccessSecret == "" {
			errs = append(errs, "COABOT_TWITTER_ACCESS_SECRET env var missing")
		}
	}

	for _, e := range errs {
		fmt.Println(e)
	}

	if len(errs) > 0 {
		os.Exit(1)
	}
}
