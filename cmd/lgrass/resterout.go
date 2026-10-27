// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
)

const (
	defaultDownloadName = "download"
	downloadTempPrefix  = ".lgrass-download-"
)

// downloadTarget is where --out points: an absolute file path, or an existing directory the file name is chosen for once the response headers are known.
type downloadTarget struct {
	path string
	dir  bool
}

// downloadResult is what a successful --out call prints. It carries no file bytes.
type downloadResult struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Size    int64               `json:"size"`
	URL     string              `json:"url"`
	SavedTo string              `json:"saved_to"`
}

// checkOutTarget validates --out before any request is made: an existing file needs confirm, and a new file needs an existing parent directory that accepts a new file.
func checkOutTarget(out string, confirm bool) (downloadTarget, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		return downloadTarget{}, fmt.Errorf("--out %q: %w", out, err)
	}
	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		if err := probeWritable(abs); err != nil {
			return downloadTarget{}, err
		}
		return downloadTarget{path: abs, dir: true}, nil
	case err == nil && !info.Mode().IsRegular():
		return downloadTarget{}, fmt.Errorf("%s is not a regular file, refusing to replace it", abs)
	case err == nil:
		if !confirm {
			return downloadTarget{}, existsError(abs, "nothing was requested")
		}
		if err := probeWritable(filepath.Dir(abs)); err != nil {
			return downloadTarget{}, err
		}
		return downloadTarget{path: abs}, nil
	case errors.Is(err, fs.ErrNotExist):
		if strings.HasSuffix(out, string(filepath.Separator)) || strings.HasSuffix(out, "/") {
			return downloadTarget{}, fmt.Errorf("the directory %s does not exist", abs)
		}
		if err := probeWritable(filepath.Dir(abs)); err != nil {
			return downloadTarget{}, err
		}
		return downloadTarget{path: abs}, nil
	default:
		return downloadTarget{}, fmt.Errorf("--out %q: %w", out, err)
	}
}

func existsError(path, consequence string) error {
	return fmt.Errorf("%s already exists, %s. Run the same command with --confirm to replace it", path, consequence)
}

// probeWritable checks that dir exists and accepts a new file by creating and removing one.
func probeWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("the directory %s is not usable: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	f, err := os.CreateTemp(dir, downloadTempPrefix+"*")
	if err != nil {
		return fmt.Errorf("cannot create a file in %s: %w", dir, err)
	}
	f.Close()
	os.Remove(f.Name())
	return nil
}

// downloadThroughAgent runs cmd's request as a download and saves a 2xx body to target. Any other response is returned as the inline result an ordinary call returns.
func downloadThroughAgent(client *agent.Client, cmd resterCommand, target downloadTarget, body []byte, contentType string) (any, error) {
	res, err := client.RequestHTTPDownload(cmd.shortID, cmd.user, cmd.method, cmd.path, body, contentType, cmd.headers)
	if err != nil {
		return nil, err
	}
	if res.Inline != nil {
		return *res.Inline, nil
	}
	return saveDownload(res, target, cmd.confirm)
}

// saveDownload streams res's body into a temporary file beside the final path and renames it into place only once the stream ended cleanly and matched its declared size, so a failed download never damages an existing file.
func saveDownload(res gatekeeper.DownloadResult, target downloadTarget, confirm bool) (downloadResult, error) {
	defer res.Body.Close()

	final := target.path
	if target.dir {
		final = filepath.Join(target.path, downloadFileName(res.Stream.Name))
	}
	if info, err := os.Lstat(final); err == nil {
		if info.IsDir() {
			return downloadResult{}, fmt.Errorf("%s is a directory, nothing was written", final)
		}
		if !confirm {
			return downloadResult{}, existsError(final, "nothing was written")
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(final), downloadTempPrefix+"*")
	if err != nil {
		return downloadResult{}, fmt.Errorf("cannot create a file in %s: %w", filepath.Dir(final), err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	defer close(done)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			os.Remove(tmpName)
			os.Exit(130)
		case <-done:
		}
	}()

	size, err := io.Copy(tmp, res.Body)
	if err != nil {
		return downloadResult{}, fmt.Errorf("the download failed after %d bytes, nothing was saved: %w", size, err)
	}
	if res.Stream.Length >= 0 && size != res.Stream.Length {
		return downloadResult{}, fmt.Errorf("received %d bytes but the server declared %d, nothing was saved", size, res.Stream.Length)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return downloadResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return downloadResult{}, err
	}
	if err := os.Rename(tmpName, final); err != nil {
		return downloadResult{}, fmt.Errorf("cannot move the download into place: %w", err)
	}
	keep = true

	return downloadResult{
		Status:  res.Stream.Status,
		Headers: res.Stream.Headers,
		Size:    size,
		URL:     res.Stream.URL,
		SavedTo: final,
	}, nil
}

func downloadFileName(name string) string {
	if name == "" {
		return defaultDownloadName
	}
	return name
}
