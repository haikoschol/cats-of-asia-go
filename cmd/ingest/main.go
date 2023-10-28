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
	"fmt"
	"github.com/haikoschol/cats-of-asia/pkg/ingestion"
	"github.com/haikoschol/cats-of-asia/pkg/postgres"
	"github.com/haikoschol/cats-of-asia/pkg/validation"
	_ "github.com/joho/godotenv/autoload"
	"googlemaps.github.io/maps"
	"log"
	"os"
)

const verbose = true // TODO make cli flag

var (
	dbHost     = os.Getenv("COA_DB_HOST")
	dbSSLMode  = os.Getenv("COA_DB_SSLMODE")
	dbName     = os.Getenv("COA_DB_NAME")
	dbUser     = os.Getenv("COA_DB_USER")
	dbPassword = os.Getenv("COA_DB_PASSWORD")

	googleMapsApiKey = os.Getenv("COA_GOOGLE_MAPS_API_KEY")
)

func main() {
	validateEnv()

	db, err := postgres.NewDatabase(dbUser, dbPassword, dbHost, dbName, postgres.SSLMode(dbSSLMode))
	if err != nil {
		log.Fatal(err)
	}

	gmapsClient, err := maps.NewClient(maps.WithAPIKey(googleMapsApiKey))
	if err != nil {
		log.Fatalf("unable to instantiate Google Maps client: %v\n", err)
	}

	i := ingestion.NewIngestor(db, gmapsClient, log.Printf, verbose)
	err = i.IngestDirectory(getImageDir())
	if err != nil {
		log.Fatal(err)
	}
}

func getImageDir() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("os.Getcwd(): %v\n", err)
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "--help" || os.Args[1] == "-h" {
			fmt.Printf("usage: %s <path> - directory to scan for new images (default: current directory)\n", os.Args[0])
			os.Exit(1)
		} else {
			dir = os.Args[1]
		}
	}
	return dir
}

func validateEnv() {
	errs := validation.ValidateDbEnv(dbHost, dbSSLMode, dbName, dbUser, dbPassword)

	if googleMapsApiKey == "" {
		errs = append(errs, "COA_GOOGLE_MAPS_API_KEY env var missing")
	}

	validation.LogErrors(errs, true)
}
