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

package mastodon

import (
	"context"
	coabot "github.com/haikoschol/cats-of-asia"
	"github.com/mattn/go-mastodon"
)

type mastodonPublisher struct {
	client *mastodon.Client
}

func New(serverUrl, accessToken string) (coabot.Publisher, error) {
	client := mastodon.NewClient(&mastodon.Config{
		Server:      serverUrl,
		AccessToken: accessToken,
	})

	return &mastodonPublisher{
		client,
	}, nil
}

func (tp mastodonPublisher) Name() string {
	return "Mastodon"
}

func (mp *mastodonPublisher) Publish(item coabot.MediaItem, description string) error {
	rc, err := item.Read()
	if err != nil {
		return err
	}
	defer rc.Close()

	media := &mastodon.Media{
		File:        rc,
		Thumbnail:   nil,
		Description: description,
	}

	attachment, err := mp.client.UploadMediaFromMedia(context.Background(), media)
	if err != nil {
		return err
	}

	toot := &mastodon.Toot{
		Status:   description,
		MediaIDs: []mastodon.ID{attachment.ID},
	}

	_, err = mp.client.PostStatus(context.Background(), toot)
	if err != nil {
		return err
	}
	return nil
}
