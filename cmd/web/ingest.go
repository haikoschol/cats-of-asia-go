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
	"context"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	coa "github.com/haikoschol/cats-of-asia"
	"github.com/haikoschol/cats-of-asia/pkg/ingestion"
	"golang.org/x/net/webdav"
	"io/fs"
	"os"
	"path"
)

type file struct {
	name     string
	path     string
	mode     os.FileMode
	created  bool
	f        webdav.File
	ingestor *ingestion.Ingestor
}

func (f *file) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *file) Readdir(count int) ([]fs.FileInfo, error) {
	return f.f.Readdir(count)
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.f.Stat()
}

func (f *file) Write(p []byte) (n int, err error) {
	return f.f.Write(p)
}

func (f *file) Close() error {
	if err := f.f.Close(); err != nil {
		return err
	}

	if f.mode.IsRegular() && f.created {
		// TODO only pass the new file to Ingestor
		// TODO offload ingestion onto a goroutine worker pool (maybe put impl in Ingestor)
		images, err := f.ingestor.IngestDirectory(f.path)
		if err != nil {
			sentry.CaptureMessage(fmt.Sprintf("failed to ingest uploaded image: %v", err))
			return err // returning an error causes the webdav request handler to respond with 404
		}

		if err := f.cleanup(images); err != nil {
			sentry.CaptureException(err)
			return err
		}
	}

	return nil
}

func (f *file) cleanup(images []coa.Image) error {
	msg := "failed to delete uploaded file %s: %w"
	// the uploaded file was already found in the database
	if len(images) == 0 {
		p := path.Join(f.path, f.name)
		if err := os.Remove(p); err != nil {
			return fmt.Errorf(msg, p, err)
		}
	} else {
		for _, img := range images {
			if err := os.Remove(img.PathLarge); err != nil {
				return fmt.Errorf(msg, img.PathLarge, err)
			}

			if err := os.Remove(img.PathMedium); err != nil {
				return fmt.Errorf(msg, img.PathMedium, err)
			}

			if err := os.Remove(img.PathSmall); err != nil {
				return fmt.Errorf(msg, img.PathSmall, err)
			}
		}
	}
	return nil
}

type fileSystem struct {
	path     string
	dir      webdav.Dir
	ingestor *ingestion.Ingestor
}

func newFileSystem(path string, ingestor *ingestion.Ingestor) *fileSystem {
	return &fileSystem{
		path:     path,
		dir:      webdav.Dir(path),
		ingestor: ingestor,
	}
}

func (fs *fileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return fs.dir.Mkdir(ctx, name, perm)
}

func (fs *fileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if !coa.IsSupportedMedia(name) {
		return nil, errors.New("unsupported file type")
	}

	wf, err := fs.dir.OpenFile(ctx, name, flag, perm)
	if err != nil {
		return nil, err
	}

	return &file{
		name:     name,
		path:     fs.path,
		mode:     perm,
		created:  flag&os.O_CREATE != 0,
		f:        wf,
		ingestor: fs.ingestor,
	}, nil
}

func (fs *fileSystem) RemoveAll(ctx context.Context, name string) error {
	return fs.dir.RemoveAll(ctx, name)
}

func (fs *fileSystem) Rename(ctx context.Context, oldName, newName string) error {
	return fs.dir.Rename(ctx, oldName, newName)
}

func (fs *fileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return fs.dir.Stat(ctx, name)
}
