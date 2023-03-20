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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/gphotosuploader/googlemirror/api/photoslibrary/v1"
	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	_ "image/jpeg"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path"
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

	state := readStateFile(statePath)
	photosClient := getPhotosClient(googlePhotosTokenPath, oauthAppCredentialsPath)
	photosService, err := photoslibrary.New(photosClient)
	if err != nil {
		log.Fatalf("Unable to create Google Photos API service: %v\n", err)
	}

	mediaItems := getMediaItemsFromAlbum(albumId, photosService)
	mediaItem := pickRandomUnusedMediaItem(mediaItems, state)

	tweetMediaItem(
		mediaItem,
		photosClient,
		twitterConsumerKey,
		twitterConsumerSecret,
		twitterAccessToken,
		twitterAccessSecret,
	)

	state.add(mediaItem)
	state.save()
}

func getMediaItemsFromAlbum(albumID string, svc *photoslibrary.Service) []*photoslibrary.MediaItem {
	searchRequest := &photoslibrary.SearchMediaItemsRequest{
		AlbumId: albumID,
	}
	searchResponse, err := svc.MediaItems.Search(searchRequest).Do()
	if err != nil {
		log.Fatalf("Unable to search for media items: %v\n", err)
	}
	if len(searchResponse.MediaItems) == 0 {
		log.Fatalf("No media items found in album %s\n", albumID)
	}
	return searchResponse.MediaItems
}

func pickRandomUnusedMediaItem(mediaItems []*photoslibrary.MediaItem, state *stateFile) *photoslibrary.MediaItem {
	unusedItems := []*photoslibrary.MediaItem{}
	for _, item := range mediaItems {
		if !state.contains(mediaId(item.Id)) {
			unusedItems = append(unusedItems, item)
		}
	}

	idx := rand.Intn(len(unusedItems))
	return unusedItems[idx]
}

func getImageFromMediaItem(mediaItem *photoslibrary.MediaItem, client *http.Client) io.ReadCloser {
	// https://developers.google.com/photos/library/guides/access-media-items#image-base-urls
	url := fmt.Sprintf("%s=d", mediaItem.BaseUrl)

	response, err := client.Get(url)
	if err != nil {
		log.Fatalf("Unable to retrieve image data from media item base URL %s: %v\n", url, err)
	}
	return response.Body // FIXME caller has to close response
}

type mediaUpload struct {
	MediaId int64 `json:"media_id"`
}

func uploadMedia(imgReader io.Reader, filename string, httpClient *http.Client) mediaUpload {
	b := &bytes.Buffer{}
	form := multipart.NewWriter(b)

	fw, err := form.CreateFormFile("media", filename)
	if err != nil {
		log.Fatalf("Unable to uhm... create a form... writer... file... thingy? %v\n", err)
	}

	_, err = io.Copy(fw, imgReader)
	if err != nil {
		log.Fatalf("Unable to copy image data into the multipart form whatever: %v\n", err)
	}
	form.Close()

	resp, err := httpClient.Post(
		"https://upload.twitter.com/1.1/media/upload.json?media_category=tweet_image",
		form.FormDataContentType(),
		bytes.NewReader(b.Bytes()),
	)
	if err != nil {
		log.Fatalf("Unable to upload image: %v\n", err)
	}
	defer resp.Body.Close()

	m := mediaUpload{}
	err = json.NewDecoder(resp.Body).Decode(&m)
	if err != nil {
		log.Fatalf("Unable to decode JSON response to media upload: %v\n", err)
	}
	return m
}

func tweetMediaItem(
	mediaItem *photoslibrary.MediaItem,
	photosClient *http.Client,
	consumerKey string,
	consumerSecret string,
	accessToken string,
	accessSecret string,
) {
	mediaReadCloser := getImageFromMediaItem(mediaItem, photosClient)
	defer mediaReadCloser.Close()

	config := oauth1.NewConfig(consumerKey, consumerSecret)
	token := oauth1.NewToken(accessToken, accessSecret)
	httpClient := config.Client(oauth1.NoContext, token)
	client := twitter.NewClient(httpClient)

	upload := uploadMedia(mediaReadCloser, mediaItem.Filename, httpClient)

	status := fmt.Sprintf("Another fine feline, captured at %s", mediaItem.MediaMetadata.CreationTime)
	_, _, err := client.Statuses.Update(status, &twitter.StatusUpdateParams{
		MediaIds: []int64{upload.MediaId},
	})
	if err != nil {
		log.Fatalf("Unable to tweet: %v\n", err)
	}
}

func getConfigFromFile(credentialsPath string) *oauth2.Config {
	if credentialsPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Unable to determine current working directory: %v\n", err)
		}

		credentialsPath = path.Join(cwd, "google_api_credentials.json")
	}

	configJSON, err := os.ReadFile(credentialsPath)
	if err != nil {
		log.Fatalf("Unable to read contents of file %s: %v\n", credentialsPath, err)
	}

	config, err := google.ConfigFromJSON(configJSON, photoslibrary.PhotoslibraryReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v\n", err)
	}

	return config
}

func getPhotosClient(tokenPath, credentialsPath string) *http.Client {
	if tokenPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Unable to determine current working directory: %v\n", err)
		}

		tokenPath = path.Join(cwd, "google_photos_token.json")
	}

	config := getConfigFromFile(credentialsPath)

	token, err := getTokenFromFile(tokenPath)
	if err != nil {
		token = getTokenFromWeb(config)
		saveToken(tokenPath, token)
	}
	return config.Client(context.Background(), token)
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

func saveToken(tokenPath string, token *oauth2.Token) {
	f, err := os.OpenFile(tokenPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache OAuth token for Google Photos API: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v\n", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v\n", err)
	}
	return tok
}

const StateFilePerms = 0644

type mediaId string

type stateItem struct {
	Filename     string
	CreationTime string
}

type stateFile struct {
	Path        string
	PostedMedia map[mediaId]stateItem
}

func (sf *stateFile) add(mi *photoslibrary.MediaItem) {
	item := stateItem{
		Filename:     mi.Filename,
		CreationTime: mi.MediaMetadata.CreationTime,
	}

	sf.PostedMedia[mediaId(mi.Id)] = item
}

func (sf *stateFile) contains(id mediaId) bool {
	_, contains := sf.PostedMedia[id]
	return contains
}

func (sf *stateFile) save() {
	f, err := os.OpenFile(sf.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, StateFilePerms)
	if err != nil {
		log.Fatalf("Unable to create/open state file at %s: %v\n", sf.Path, err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(sf); err != nil {
		log.Fatalf("Unable to write state file at %s: %v\n", sf.Path, err)
	}
}

func readStateFile(spath string) *stateFile {
	fileExists := true
	info, err := os.Stat(spath)
	if errors.Is(err, os.ErrNotExist) {
		fileExists = false
	} else if err != nil {
		log.Fatalf("Unable to stat file at %s: %v\n", spath, err)
	}

	if info != nil && info.IsDir() {
		log.Fatalf("Unable to create state file because there is a directory with the same name at %s\n", spath)
	}

	f, err := os.OpenFile(spath, os.O_CREATE|os.O_RDWR, StateFilePerms)
	if err != nil {
		log.Fatalf("Unable to create/open state file at %s: %v\n", spath, err)
	}
	defer f.Close()

	state := &stateFile{
		Path:        spath,
		PostedMedia: map[mediaId]stateItem{},
	}

	if fileExists {
		if err := json.NewDecoder(f).Decode(state); err != nil {
			log.Fatalf("Unable to unmarshal state file at %s from JSON: %v\n", spath, err)
		}
	} else {
		state.save()
	}

	return state
}

func validateEnv() {
	if albumId == "" {
		log.Fatalf("COABOT_ALBUM_ID env var missing\n")
	}
	if statePath == "" {
		log.Fatalf("COABOT_STATE_FILE env var missing\n")
	}
	if oauthAppCredentialsPath == "" {
		log.Fatalf("COABOT_OAUTH_APP_CREDENTIALS_FILE env var missing\n")
	}
	if googlePhotosTokenPath == "" {
		log.Fatalf("COABOT_GOOGLE_PHOTOS_TOKEN_FILE env var missing\n")
	}
	if twitterConsumerKey == "" {
		log.Fatalf("COABOT_TWITTER_CONSUMER_KEY env var missing\n")
	}
	if twitterConsumerSecret == "" {
		log.Fatalf("COABOT_TWITTER_CONSUMER_SECRET env var missing\n")
	}
	if twitterAccessToken == "" {
		log.Fatalf("COABOT_TWITTER_ACCESS_TOKEN env var missing\n")
	}
	if twitterAccessSecret == "" {
		log.Fatalf("COABOT_TWITTER_ACCESS_SECRET env var missing\n")
	}
}
