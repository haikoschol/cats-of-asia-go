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

package google_maps_geocoder

import (
	"context"
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	"googlemaps.github.io/maps"
)

type googleMapsGeocoder struct {
	client *maps.Client
}

func New(apiKey string) (coabot.Geocoder, error) {
	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("unable to create Google Maps API client: %w", err)
	}
	return &googleMapsGeocoder{
		client,
	}, nil
}

func (g *googleMapsGeocoder) LookupCityAndCountry(latitude, longitude float64) (coabot.CityAndCountry, error) {
	r := &maps.GeocodingRequest{
		LatLng: &maps.LatLng{
			Lat: latitude,
			Lng: longitude,
		},
	}

	locs, err := g.client.ReverseGeocode(context.Background(), r)
	if err != nil {
		return coabot.CityAndCountry{}, err
	}

	if len(locs) == 0 || len(locs[0].AddressComponents) == 0 {
		return coabot.CityAndCountry{}, fmt.Errorf(
			"the Google Maps API did not return required address components for latitude %f, longitude %f",
			latitude,
			longitude,
		)
	}

	loc := coabot.CityAndCountry{}

	for _, comp := range locs[0].AddressComponents {
		for _, t := range comp.Types {
			if t == "administrative_area_level_1" {
				loc.City = comp.LongName
			} else if t == "country" {
				loc.Country = comp.LongName
			}
			if loc.City != "" && loc.Country != "" {
				break
			}
		}
	}

	if loc.City == "" || loc.Country == "" {
		return loc, fmt.Errorf("couldn't find either city or country for coordinates %f, %f", latitude, longitude)
	}
	return loc, nil
}
