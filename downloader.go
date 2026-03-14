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

	formats := []string{
		"bv*+ba/b",
		"best",
		"bestvideo+bestaudio",
	}

	var lastErr error
	for _, format := range formats {
		args := []string{
			"--no-playlist",
			"--no-progress",
			"--restrict-filenames",
			"--output", outputTemplate,
			"--format", format,
			"--merge-output-format", "mp4",
			"--sleep-requests", fmt.Sprintf("%d", cfg.YtDLPSleepSeconds),
			"--sleep-interval", fmt.Sprintf("%d", cfg.YtDLPSleepSeconds),
			"--max-sleep-interval", fmt.Sprintf("%d", cfg.YtDLPSleepSeconds+2),
		}
		if len(cfg.YtDLPExtraArgs) > 0 {
			args = append(args, cfg.YtDLPExtraArgs...)
		}
		args = append(args, rawURL)
		cmd := exec.CommandContext(ctx, cfg.YtDLPBinary, args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		output, err := cmd.CombinedOutput()
		if err == nil {
			matches, globErr := filepath.Glob(targetBase + ".*")
			if globErr != nil || len(matches) == 0 {
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

		normalizedErr := normalizeYtDLPError(err, output)
		if !isFormatAvailabilityError(string(output)) {
			return DownloadResult{}, normalizedErr
		}
		lastErr = normalizedErr
	}

	if lastErr != nil {
		return DownloadResult{}, lastErr
	}
	return DownloadResult{}, fmt.Errorf("yt-dlp failed: no supported format could be downloaded")
}

func normalizeYtDLPError(err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	switch {
	case strings.Contains(message, "Only images are available for download"):
		return fmt.Errorf("yt-dlp failed: %w: YouTube did not expose downloadable video streams for this link, only images/storyboards were available", err)
	case strings.Contains(message, "Requested format is not available"):
		return fmt.Errorf("yt-dlp failed: %w: YouTube did not provide a usable video format for this link", err)
	default:
		return fmt.Errorf("yt-dlp failed: %w: %s", err, message)
	}
}

func isFormatAvailabilityError(message string) bool {
	return strings.Contains(message, "Requested format is not available") ||
		strings.Contains(message, "Only images are available for download")
}
