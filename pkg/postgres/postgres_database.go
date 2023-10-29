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

package postgres

import (
	"database/sql"
	"fmt"
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/lib/pq"
	"net/url"
	"time"
)

type SSLMode string

const (
	VerifyFull SSLMode = "verify-full"
	VerifyCA           = "verify-ca"
	Disable            = "disable"
)

type pgDatabase struct {
	db *sql.DB
}

func NewDatabase(dbUser, dbPassword, dbHost, dbName string, dbSSLMode SSLMode) (coa.Database, error) {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", dbUser, dbPassword, dbHost, dbName, dbSSLMode)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	return &pgDatabase{db}, nil
}

func (d *pgDatabase) GetOrCreateLocation(city, country, timezone string) (int64, error) {
	_, err := d.db.Exec(
		`INSERT INTO
    			locations(city, country, timezone)
			VALUES
			    ($1, $2, $3)
			ON CONFLICT (city, country) DO NOTHING`,
		city,
		country,
		timezone,
	)
	if err != nil {
		return 0, err
	}

	row := d.db.QueryRow("SELECT id FROM locations WHERE city = $1 and country = $2", city, country)
	var id int64
	err = row.Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (d *pgDatabase) GetOrCreateCoordinates(latitude, longitude float64, locationId int64) (int64, error) {
	_, err := d.db.Exec(
		`INSERT INTO
    			coordinates(latitude, longitude, location_id)
			VALUES
			    ($1, $2, $3)
			ON CONFLICT (latitude, longitude) DO NOTHING`,
		latitude,
		longitude,
		locationId,
	)
	if err != nil {
		return 0, err
	}

	row := d.db.QueryRow("SELECT id FROM coordinates WHERE latitude = $1 and longitude = $2", latitude, longitude)
	var id int64
	err = row.Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (d *pgDatabase) GetCoordinateID(latitude, longitude float64) (int64, error) {
	row := d.db.QueryRow("SELECT id FROM coordinates WHERE latitude = $1 AND longitude = $2", latitude, longitude)
	var id int64
	err := row.Scan(&id)
	return id, err
}

func (d *pgDatabase) GetImage(id int64) (coa.Image, error) {
	row := d.db.QueryRow(`
		SELECT 
			i.id AS image_id,
			i.url_large,
			i.url_medium,
			i.url_small,
			i.sha256,
			i.timestamp,
			c.latitude,
			c.longitude,
			l.city,
			l.country,
			l.timezone
		FROM images AS i
		JOIN coordinates AS c ON i.coordinate_id = c.id
		JOIN locations AS l ON c.location_id = l.id
		WHERE i.id = $1`,
		id)

	var img coa.Image
	var ul, um, us string

	err := row.Scan(
		&img.ID,
		&ul,
		&um,
		&us,
		&img.SHA256,
		&img.Timestamp,
		&img.Latitude,
		&img.Longitude,
		&img.City,
		&img.Country,
		&img.Timezone)

	if err != nil {
		return img, err
	}

	img.URLLarge, err = url.Parse(ul)
	if err != nil {
		return img, err
	}

	img.URLMedium, err = url.Parse(um)
	if err != nil {
		return img, err
	}

	img.URLSmall, err = url.Parse(us)
	if err != nil {
		return img, err
	}

	return fixTimezone(img)
}

func (d *pgDatabase) GetImages() ([]coa.Image, error) {
	rows, err := d.db.Query(`
		SELECT 
			i.id AS image_id,
			i.url_large,
			i.url_medium,
			i.url_small,
			i.sha256,
			i.timestamp,
			c.latitude,
			c.longitude,
			l.city,
			l.country,
			l.timezone
		FROM images AS i
		JOIN coordinates AS c ON i.coordinate_id = c.id
		JOIN locations AS l ON c.location_id = l.id`)

	if err != nil {
		return nil, err
	}

	var images []coa.Image
	var ul, um, us string

	for rows.Next() {
		var img coa.Image

		err := rows.Scan(
			&img.ID,
			&ul,
			&um,
			&us,
			&img.SHA256,
			&img.Timestamp,
			&img.Latitude,
			&img.Longitude,
			&img.City,
			&img.Country,
			&img.Timezone)

		if err != nil {
			return nil, err
		}

		img.URLLarge, err = url.Parse(ul)
		if err != nil {
			return nil, err
		}

		img.URLMedium, err = url.Parse(um)
		if err != nil {
			return nil, err
		}

		img.URLSmall, err = url.Parse(us)
		if err != nil {
			return nil, err
		}

		img, err = fixTimezone(img)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}

	return images, nil
}

func (d *pgDatabase) GetRandomUnusedImage(platform coa.Platform) (coa.Image, error) {
	row := d.db.QueryRow(`
		SELECT 
			i.id AS image_id,
			i.url_large,
			i.url_medium,
			i.url_small,
			i.sha256,
			i.timestamp,
			c.latitude,
			c.longitude,
			l.city,
			l.country,
			l.timezone
		FROM images AS i
		JOIN coordinates AS c ON i.coordinate_id = c.id
		JOIN locations AS l ON c.location_id = l.id
		WHERE i.id NOT IN (
			SELECT image_id FROM posts where platform_id = (SELECT id FROM platforms WHERE name = $1)
	    )
		ORDER BY random()
		LIMIT 1;`,
		platform)

	var img coa.Image
	var ul, um, us string

	err := row.Scan(
		&img.ID,
		&ul,
		&um,
		&us,
		&img.SHA256,
		&img.Timestamp,
		&img.Latitude,
		&img.Longitude,
		&img.City,
		&img.Country,
		&img.Timezone)

	if err != nil {
		return img, err
	}

	img.URLLarge, err = url.Parse(ul)
	if err != nil {
		return img, err
	}

	img.URLMedium, err = url.Parse(um)
	if err != nil {
		return img, err
	}

	img.URLSmall, err = url.Parse(us)
	if err != nil {
		return img, err
	}

	return fixTimezone(img)
}

func (d *pgDatabase) GetUnusedImageCount(platform coa.Platform) (int, error) {
	row := d.db.QueryRow(`
		SELECT 
			COUNT(id)
		FROM images
		WHERE id NOT IN (
			SELECT image_id FROM posts where platform_id = (SELECT id FROM platforms WHERE name = $1)
	    )
		ORDER BY random()
		LIMIT 1;`,
		platform)

	var count int
	err := row.Scan(&count)
	return count, err
}

func (d *pgDatabase) RemoveKnownImages(images []coa.Image) ([]coa.Image, error) {
	var hashes []string

	for _, img := range images {
		hashes = append(hashes, img.SHA256)
	}

	rows, err := d.db.Query(`SELECT url_large, sha256 FROM images WHERE sha256 = ANY($1)`, pq.Array(hashes))
	if err != nil {
		return nil, err
	}

	knownImages := make(map[string]string)

	for rows.Next() {
		var imgPath, hash string
		err = rows.Scan(&imgPath, &hash)
		if err != nil {
			return nil, err
		}
		knownImages[hash] = imgPath
	}

	var filtered []coa.Image

	for _, img := range images {
		_, ok := knownImages[img.SHA256]
		if ok {
			continue
		}

		filtered = append(filtered, img)
	}

	return filtered, nil
}

func (d *pgDatabase) InsertImages(images []coa.Image) error {
	for _, img := range images {
		if img.CoordinateID == nil {
			locId, err := d.GetOrCreateLocation(img.City, img.Country, img.Timezone)
			if err != nil {
				return err
			}

			coordId, err := d.GetOrCreateCoordinates(img.Latitude, img.Longitude, locId)
			if err != nil {
				return err
			}

			img.CoordinateID = &coordId
		}
		_, err := d.db.Exec(
			`INSERT INTO
    			images(url_large, url_medium, url_small, sha256, timestamp, coordinate_id)
			VALUES
			    ($1, $2, $3, $4, $5, $6)`,
			img.URLLarge.String(),
			img.URLMedium.String(),
			img.URLSmall.String(),
			img.SHA256,
			img.Timestamp,
			img.CoordinateID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *pgDatabase) InsertPost(image coa.Image, platform coa.Platform) error {
	row := d.db.QueryRow("SELECT id FROM platforms WHERE name = $1", platform)
	var pID int64
	err := row.Scan(&pID)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(
		`INSERT INTO
    			posts(image_id, platform_id)
			VALUES
			    ($1, $2)`,
		image.ID,
		pID,
	)
	if err != nil {
		return err
	}
	return nil
}

func fixTimezone(image coa.Image) (coa.Image, error) {
	loc, err := time.LoadLocation(image.Timezone)
	if err != nil {
		return image, err
	}

	image.Timestamp = image.Timestamp.In(loc)
	return image, nil
}
