package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	BotToken          string
	OwnerUserID       int64
	OwnerUsername     string
	OwnerUsernames    map[string]struct{}
	ChannelID         string
	DataDir           string
	TempDir           string
	YtDLPBinary       string
	YtDLPExtraArgs    []string
	YtDLPSleepSeconds int
	FFmpegBinary      string
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:          strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		OwnerUsername:     normalizeTelegramUsername(os.Getenv("OWNER_USERNAME")),
		OwnerUsernames:    splitUsernames(os.Getenv("OWNER_USERNAMES")),
		ChannelID:         strings.TrimSpace(os.Getenv("CHANNEL_ID")),
		DataDir:           defaultString(os.Getenv("DATA_DIR"), "data"),
		TempDir:           defaultString(os.Getenv("TEMP_DIR"), "data/tmp"),
		YtDLPBinary:       defaultString(os.Getenv("YT_DLP_BINARY"), "yt-dlp"),
		YtDLPExtraArgs:    splitArgs(os.Getenv("YT_DLP_EXTRA_ARGS")),
		YtDLPSleepSeconds: defaultInt(os.Getenv("YT_DLP_SLEEP_SECONDS"), 3),
		FFmpegBinary:      defaultString(os.Getenv("FFMPEG_BINARY"), "ffmpeg"),
	}

	ownerRaw := strings.TrimSpace(os.Getenv("OWNER_USER_ID"))
	if cfg.BotToken == "" {
		return cfg, fmt.Errorf("BOT_TOKEN is required")
	}
	if ownerRaw == "" && cfg.OwnerUsername == "" && len(cfg.OwnerUsernames) == 0 {
		return cfg, fmt.Errorf("OWNER_USER_ID, OWNER_USERNAME, or OWNER_USERNAMES is required")
	}
	if ownerRaw != "" {
		ownerID, err := strconv.ParseInt(ownerRaw, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("OWNER_USER_ID must be int64: %w", err)
		}
		cfg.OwnerUserID = ownerID
	}
	if cfg.ChannelID == "" {
		return cfg, fmt.Errorf("CHANNEL_ID is required")
	}

	cfg.DataDir = filepath.Clean(cfg.DataDir)
	cfg.TempDir = filepath.Clean(cfg.TempDir)
	return cfg, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizeTelegramUsername(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "@")
	return strings.ToLower(value)
}

func splitArgs(value string) []string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func splitUsernames(value string) map[string]struct{} {
	items := strings.Split(strings.TrimSpace(value), ",")
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		username := normalizeTelegramUsername(item)
		if username == "" {
			continue
		}
		result[username] = struct{}{}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
