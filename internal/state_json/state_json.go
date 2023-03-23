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

package state_json

import (
	"encoding/json"
	"errors"
	"fmt"
	coabot "github.com/haikoschol/cats-of-asia"
	"os"
)

const stateFilePermissions = 0644

type stateId string

type stateJSONFile struct {
	Path       string
	MediaItems map[stateId]coabot.MediaItem
}

// New returns a coabot.ApplicationState implementation backed by a JSON file
func New(spath string) (coabot.ApplicationState, error) {
	fileExists := true
	info, err := os.Stat(spath)
	if errors.Is(err, os.ErrNotExist) {
		fileExists = false
	} else if err != nil {
		return nil, fmt.Errorf("unable to stat file at %s: %w", spath, err)
	}

	if info != nil && info.IsDir() {
		return nil, fmt.Errorf("a directory already exists at %s", spath)
	}

	state := &stateJSONFile{
		Path:       spath,
		MediaItems: map[stateId]coabot.MediaItem{},
	}

	if !fileExists {
		return state, state.save()
	}

	f, err := os.Open(spath)
	if err != nil {
		return nil, fmt.Errorf("unable to open state file at %s: %w", spath, err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(state); err != nil {
		return nil, fmt.Errorf("unable to unmarshal state file at %s from JSON: %w", spath, err)
	}

	return state, nil
}

func (sf *stateJSONFile) Add(item coabot.MediaItem) error {
	id := sf.id(item)
	sf.MediaItems[id] = item
	return sf.save()
}

func (sf *stateJSONFile) Contains(item coabot.MediaItem) bool {
	id := sf.id(item)
	_, contains := sf.MediaItems[id]
	return contains
}

func (sf *stateJSONFile) save() error {
	f, err := os.OpenFile(sf.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, stateFilePermissions)
	if err != nil {
		return fmt.Errorf("unable to create/open state file at %s: %w", sf.Path, err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(sf); err != nil {
		return fmt.Errorf("unable to write JSON state file at %s: %w", sf.Path, err)
	}
	return nil
}

func (sf *stateJSONFile) id(item coabot.MediaItem) stateId {
	return stateId(item.AlbumId + item.Id)
}
