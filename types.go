package main

import "time"

type UpdateResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type APIResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type GetFileResponse struct {
	OK     bool         `json:"ok"`
	Result TelegramFile `json:"result"`
}

type SendMessageResponse struct {
	OK     bool    `json:"ok"`
	Result Message `json:"result"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID           int64          `json:"message_id"`
	From                *User          `json:"from"`
	Chat                Chat           `json:"chat"`
	Text                string         `json:"text"`
	Caption             string         `json:"caption"`
	Photo               []PhotoSize    `json:"photo"`
	Video               *Video         `json:"video"`
	Document            *Document      `json:"document"`
	ReplyToMessage      *Message       `json:"reply_to_message"`
	ForwardOrigin       *ForwardOrigin `json:"forward_origin"`
	HasProtectedContent bool           `json:"has_protected_content"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size"`
}

type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
}

type Document struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type TelegramFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size"`
}

type ForwardOrigin struct {
	Type string `json:"type"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type DraftStatus string

const (
	DraftReady     DraftStatus = "ready"
	DraftPublished DraftStatus = "published"
	DraftFailed    DraftStatus = "failed"
)

type MediaKind string

const (
	MediaPhoto    MediaKind = "photo"
	MediaVideo    MediaKind = "video"
	MediaDocument MediaKind = "document"
)

type Draft struct {
	ID            string      `json:"id"`
	OwnerUserID   int64       `json:"owner_user_id"`
	Status        DraftStatus `json:"status"`
	MediaKind     MediaKind   `json:"media_kind"`
	LocalPath     string      `json:"local_path"`
	OriginalName  string      `json:"original_name"`
	SourceURL     string      `json:"source_url,omitempty"`
	SourceLabel   string      `json:"source_label,omitempty"`
	Caption       string      `json:"caption,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	PublishedAt   *time.Time  `json:"published_at,omitempty"`
	ErrorMessage  string      `json:"error_message,omitempty"`
	OriginSummary string      `json:"origin_summary,omitempty"`
}
