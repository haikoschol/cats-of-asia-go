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

package google_photos

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gphotosuploader/googlemirror/api/photoslibrary/v1"
	coabot "github.com/haikoschol/cats-of-asia"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"log"
	"net/http"
	"os"
	"time"
)

type googlePhotosClient struct {
	albumId string
	config  *oauth2.Config
	token   *oauth2.Token
	client  *http.Client
	service *photoslibrary.Service
}

func New(albumId, oauthAppCredentialsPath, googlePhotosTokenPath string) (coabot.MediaAlbum, error) {
	config, err := getConfigFromFile(oauthAppCredentialsPath)
	if err != nil {
		return nil, err
	}

	token, err := getTokenFromFile(googlePhotosTokenPath)
	if err != nil {
		token, err = getTokenFromWeb(config)
		if err := saveToken(googlePhotosTokenPath, token); err != nil {
			return nil, err
		}
	}

	client := config.Client(context.Background(), token)

	service, err := photoslibrary.New(client)
	if err != nil {
		return nil, fmt.Errorf("unable to create Google Photos API service: %w", err)
	}

	gpClient := googlePhotosClient{
		albumId,
		config,
		token,
		client,
		service,
	}

	return gpClient, nil
}

func (gpc googlePhotosClient) Id() string {
	return gpc.albumId
}

func (gpc googlePhotosClient) GetMediaItems() ([]coabot.MediaItem, error) {
	searchRequest := &photoslibrary.SearchMediaItemsRequest{
		AlbumId: gpc.Id(),
	}
	searchResponse, err := gpc.service.MediaItems.Search(searchRequest).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to search for media items: %w", err)
	}

	if len(searchResponse.MediaItems) == 0 {
		return nil, fmt.Errorf("no media items found in album %s", gpc.Id())
	}

	mediaItems := make([]coabot.MediaItem, len(searchResponse.MediaItems))

	for i, item := range searchResponse.MediaItems {
		creationTime, err := time.Parse(time.RFC3339, item.MediaMetadata.CreationTime)
		if err != nil {
			return nil, fmt.Errorf(
				"unable to parse creation time %s in file %s: %w",
				item.MediaMetadata.CreationTime,
				item.Filename,
				err,
			)
		}

		mediaItems[i] = coabot.MediaItem{
			Id:           item.Id,
			AlbumId:      gpc.Id(),
			Filename:     item.Filename,
			CreationTime: creationTime,
			Latitude:     -1.0, // sadness https://issuetracker.google.com/issues/80379228
			Longitude:    -1.0,
			BaseUrl:      item.BaseUrl,
			Category:     coabot.Photo, // TODO support video
		}
	}
	return mediaItems, nil
}

func (gpc googlePhotosClient) GetContentFromMediaItem(item coabot.MediaItem) (coabot.MediaContent, error) {
	// https://developers.google.com/photos/library/guides/access-media-items#image-base-urls
	url := fmt.Sprintf("%s=d", item.BaseUrl)

	response, err := gpc.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve media content from media item base URL %s: %w", url, err)
	}
	defer response.Body.Close()

	content := make([]byte, response.ContentLength)
	_, err = response.Body.Read(content)
	if err != nil {
		return nil, fmt.Errorf("unable to read media content from response body: %w", err)
	}
	return content, nil
}

func getConfigFromFile(credentialsPath string) (*oauth2.Config, error) {
	configJSON, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read contents of credentials file %s: %w", credentialsPath, err)
	}

	config, err := google.ConfigFromJSON(configJSON, photoslibrary.PhotoslibraryReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	return config, nil
}

func getTokenFromFile(tokenPath string) (*oauth2.Token, error) {
	f, err := os.Open(tokenPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

func saveToken(tokenPath string, token *oauth2.Token) error {
	f, err := os.OpenFile(tokenPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to open file for caching Google Photos OAuth token: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("unable to write file for caching Google Photos OAuth token: %w", err)
	}
	return nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code for Google Photos OAuth token: %w", err)
	}

	token, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Google Photos OAuth token from web: %w", err)
	}
	return token, nil
}
