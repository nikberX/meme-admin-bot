package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DownloadResult struct {
	Path         string
	MediaKind    MediaKind
	OriginalName string
}

func DownloadByURL(ctx context.Context, cfg Config, store *Store, rawURL string) (DownloadResult, error) {
	if _, err := exec.LookPath(cfg.YtDLPBinary); err != nil {
		return DownloadResult{}, fmt.Errorf("yt-dlp is required for URL imports; install it or set YT_DLP_BINARY: %w", err)
	}

	draftID := store.NextDraftID()
	targetBase := filepath.Join(cfg.TempDir, draftID)
	outputTemplate := targetBase + ".%(ext)s"

	args := []string{
		"--no-playlist",
		"--no-progress",
		"--restrict-filenames",
		"--output", outputTemplate,
		"--format", "bv*+ba/b",
		"--merge-output-format", "mp4",
		rawURL,
	}
	cmd := exec.CommandContext(ctx, cfg.YtDLPBinary, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return DownloadResult{}, fmt.Errorf("yt-dlp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	matches, err := filepath.Glob(targetBase + ".*")
	if err != nil || len(matches) == 0 {
		return DownloadResult{}, fmt.Errorf("download finished but file not found")
	}

	path := matches[0]
	kind := DetectMediaKind(path, "")
	return DownloadResult{
		Path:         path,
		MediaKind:    kind,
		OriginalName: filepath.Base(path),
	}, nil
}
