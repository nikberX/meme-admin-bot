package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	BotToken     string
	OwnerUserID  int64
	ChannelID    string
	DataDir      string
	TempDir      string
	YtDLPBinary  string
	FFmpegBinary string
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:     strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		ChannelID:    strings.TrimSpace(os.Getenv("CHANNEL_ID")),
		DataDir:      defaultString(os.Getenv("DATA_DIR"), "data"),
		TempDir:      defaultString(os.Getenv("TEMP_DIR"), "data/tmp"),
		YtDLPBinary:  defaultString(os.Getenv("YT_DLP_BINARY"), "yt-dlp"),
		FFmpegBinary: defaultString(os.Getenv("FFMPEG_BINARY"), "ffmpeg"),
	}

	ownerRaw := strings.TrimSpace(os.Getenv("OWNER_USER_ID"))
	if cfg.BotToken == "" {
		return cfg, fmt.Errorf("BOT_TOKEN is required")
	}
	if ownerRaw == "" {
		return cfg, fmt.Errorf("OWNER_USER_ID is required")
	}
	ownerID, err := strconv.ParseInt(ownerRaw, 10, 64)
	if err != nil {
		return cfg, fmt.Errorf("OWNER_USER_ID must be int64: %w", err)
	}
	cfg.OwnerUserID = ownerID
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
