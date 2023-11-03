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

package twitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	coa "github.com/haikoschol/cats-of-asia"
	"mime/multipart"
	"net/http"
)

type twitterPublisher struct {
	creds      Credentials
	config     *oauth1.Config
	token      *oauth1.Token
	httpClient *http.Client
	client     *twitter.Client
}

type Credentials struct {
	ConsumerKey    string
	ConsumerSecret string
	AccessToken    string
	AccessSecret   string
}

func NewPublisher(creds Credentials) coa.Publisher {
	config := oauth1.NewConfig(creds.ConsumerKey, creds.ConsumerSecret)
	token := oauth1.NewToken(creds.AccessToken, creds.AccessSecret)
	httpClient := config.Client(oauth1.NoContext, token)
	client := twitter.NewClient(httpClient)

	return twitterPublisher{
		creds,
		config,
		token,
		httpClient,
		client,
	}
}

func (tp twitterPublisher) Platform() coa.Platform {
	return coa.X
}

func (tp twitterPublisher) Publish(image coa.Image, description string) error {
	upload, err := tp.upload(image)

	_, _, err = tp.client.Statuses.Update(description, &twitter.StatusUpdateParams{
		MediaIds: []int64{upload.MediaId},
	})
	if err != nil {
		return fmt.Errorf("tweeting failed: %w", err)
	}

	return nil
}

type upload struct {
	MediaId int64 `json:"media_id"`
}

func (tp twitterPublisher) upload(image coa.Image) (*upload, error) {
	b := &bytes.Buffer{}
	form := multipart.NewWriter(b)

	fw, err := form.CreateFormFile("media", image.Name())
	if err != nil {
		return nil, fmt.Errorf("unable to encode media for upload to Twitter: %w", err)
	}

	content, err := image.Content()
	if err != nil {
		return nil, err
	}

	if _, err := fw.Write(content); err != nil {
		return nil, fmt.Errorf("unable to copy media content into the multipart form: %w", err)
	}

	if err := form.Close(); err != nil {
		return nil, fmt.Errorf("unable to close the multipart form: %w", err)
	}

	response, err := tp.httpClient.Post(
		"https://upload.twitter.com/1.1/media/upload.json?media_category=tweet_image",
		form.FormDataContentType(),
		bytes.NewReader(b.Bytes()),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to upload media to Twitter: %w", err)
	}
	defer response.Body.Close()

	m := &upload{}
	err = json.NewDecoder(response.Body).Decode(m)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response to Twitter media upload: %w", err)
	}
	return m, nil
}
