package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type inlineKeyboardMarkup struct {
	InlineKeyboard [][]inlineKeyboardButton `json:"inline_keyboard"`
}

type inlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type Bot struct {
	cfg    Config
	store  *Store
	logger *log.Logger
	client *http.Client
	apiURL string
}

func NewBot(cfg Config, store *Store, logger *log.Logger) *Bot {
	return &Bot{
		cfg:    cfg,
		store:  store,
		logger: logger,
		client: &http.Client{Timeout: 5 * time.Minute},
		apiURL: fmt.Sprintf("https://api.telegram.org/bot%s", cfg.BotToken),
	}
}

func (b *Bot) Run(ctx context.Context) error {
	var offset int64
	nextCleanup := time.Now().UTC()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			b.logger.Printf("getUpdates error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.CallbackQuery != nil {
				if err := b.handleCallbackQuery(ctx, *update.CallbackQuery); err != nil {
					b.logger.Printf("handle callback %d: %v", update.UpdateID, err)
				}
				continue
			}
			if update.Message == nil {
				continue
			}
			if err := b.handleMessage(ctx, *update.Message); err != nil {
				b.logger.Printf("handle update %d: %v", update.UpdateID, err)
			}
		}

		if time.Now().UTC().After(nextCleanup) {
			if err := b.cleanupDraftMedia(); err != nil {
				b.logger.Printf("cleanup error: %v", err)
			}
			nextCleanup = time.Now().UTC().Add(1 * time.Hour)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg Message) error {
	if msg.From == nil {
		return nil
	}
	if !b.isOwner(*msg.From) {
		return b.sendText(ctx, msg.Chat.ID, "Этот бот принимает команды только от владельца.")
	}

	text := strings.TrimSpace(msg.Text)
	switch {
	case text == "/start" || text == "/help":
		return b.sendHelp(ctx, msg.Chat.ID)
	case text == "/list":
		return b.sendDraftList(ctx, msg.Chat.ID, msg.From.ID)
	case strings.HasPrefix(text, "/caption "):
		return b.handleSetCaption(ctx, msg)
	case strings.HasPrefix(text, "/source "):
		return b.handleSetSource(ctx, msg)
	case strings.HasPrefix(text, "/publish "):
		return b.handlePublish(ctx, msg)
	case strings.HasPrefix(text, "/delete "):
		return b.handleDelete(ctx, msg)
	}

	if urlValue := firstURL(text, msg.Caption); urlValue != "" {
		return b.handleURLDraft(ctx, msg, urlValue)
	}

	if hasMedia(msg) {
		return b.handleTelegramMediaDraft(ctx, msg)
	}

	return b.sendText(ctx, msg.Chat.ID, "Не понял сообщение. Отправь ссылку, фото, видео или документ с медиа. /help")
}

func (b *Bot) handleCallbackQuery(ctx context.Context, cb CallbackQuery) error {
	if cb.From == nil {
		return nil
	}
	if !b.isOwner(*cb.From) {
		return b.answerCallbackQuery(ctx, cb.ID, "Недостаточно прав")
	}
	switch {
	case strings.HasPrefix(cb.Data, "publish:"):
		draftID := strings.TrimPrefix(cb.Data, "publish:")
		if err := b.publishDraftByID(ctx, draftID); err != nil {
			_ = b.answerCallbackQuery(ctx, cb.ID, "Ошибка публикации")
			if cb.Message != nil {
				return b.sendText(ctx, cb.Message.Chat.ID, "Публикация не удалась: "+err.Error())
			}
			return err
		}
		return b.answerCallbackQuery(ctx, cb.ID, "Опубликовано")
	case strings.HasPrefix(cb.Data, "discard:"):
		draftID := strings.TrimPrefix(cb.Data, "discard:")
		if err := b.discardDraftByID(draftID); err != nil {
			_ = b.answerCallbackQuery(ctx, cb.ID, "Ошибка удаления")
			if cb.Message != nil {
				return b.sendText(ctx, cb.Message.Chat.ID, "Не удалось удалить черновик: "+err.Error())
			}
			return err
		}
		if cb.Message != nil {
			if err := b.deleteMessage(ctx, cb.Message.Chat.ID, cb.Message.MessageID); err != nil {
				b.logger.Printf("delete draft message: %v", err)
			}
		}
		return b.answerCallbackQuery(ctx, cb.ID, "Черновик удален")
	default:
		return b.answerCallbackQuery(ctx, cb.ID, "Неизвестное действие")
	}
}

func (b *Bot) isOwner(user User) bool {
	if b.cfg.OwnerUserID != 0 && user.ID == b.cfg.OwnerUserID {
		return true
	}
	username := normalizeTelegramUsername(user.Username)
	if b.cfg.OwnerUsername != "" && username == b.cfg.OwnerUsername {
		return true
	}
	if _, ok := b.cfg.OwnerUsernames[username]; ok {
		return true
	}
	return false
}

func (b *Bot) handleURLDraft(ctx context.Context, msg Message, rawURL string) error {
	if err := b.sendText(ctx, msg.Chat.ID, "Скачиваю медиа по ссылке, это может занять до пары минут."); err != nil {
		return err
	}
	result, err := DownloadByURL(ctx, b.cfg, b.store, rawURL)
	if err != nil {
		return b.sendText(ctx, msg.Chat.ID, "Не удалось скачать ссылку: "+err.Error())
	}

	finalPath, err := b.moveIntoStore(result.Path, result.OriginalName)
	if err != nil {
		return b.sendText(ctx, msg.Chat.ID, "Не удалось сохранить файл: "+err.Error())
	}

	draft := Draft{
		ID:            b.store.NextDraftID(),
		OwnerUserID:   msg.From.ID,
		Status:        DraftReady,
		MediaKind:     result.MediaKind,
		LocalPath:     finalPath,
		OriginalName:  filepath.Base(finalPath),
		SourceURL:     rawURL,
		CreatedAt:     time.Now().UTC(),
		OriginSummary: "downloaded from URL",
	}
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return b.sendTextWithKeyboard(ctx, msg.Chat.ID, formatDraftReadyMessage(draft), draftKeyboard(draft))
}

func (b *Bot) handleTelegramMediaDraft(ctx context.Context, msg Message) error {
	if msg.HasProtectedContent {
		return b.sendText(ctx, msg.Chat.ID, "У сообщения включена защита контента, Telegram не дает боту сохранить такое медиа.")
	}

	fileID, originalName, kind := extractMedia(msg)
	if fileID == "" {
		return b.sendText(ctx, msg.Chat.ID, "Поддерживаются фото, видео и документы с image/video mime-type.")
	}

	path, err := b.downloadTelegramFile(ctx, fileID, originalName)
	if err != nil {
		return b.sendText(ctx, msg.Chat.ID, "Не удалось скачать медиа из Telegram: "+err.Error())
	}

	draft := Draft{
		ID:            b.store.NextDraftID(),
		OwnerUserID:   msg.From.ID,
		Status:        DraftReady,
		MediaKind:     kind,
		LocalPath:     path,
		OriginalName:  filepath.Base(path),
		Caption:       strings.TrimSpace(msg.Caption),
		CreatedAt:     time.Now().UTC(),
		OriginSummary: describeOrigin(msg),
	}
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return b.sendTextWithKeyboard(ctx, msg.Chat.ID, formatDraftReadyMessage(draft), draftKeyboard(draft))
}

func (b *Bot) handleSetCaption(ctx context.Context, msg Message) error {
	id, value, ok := splitCommandArg(msg.Text)
	if !ok {
		return b.sendText(ctx, msg.Chat.ID, "Формат: /caption <draft_id> <текст>")
	}
	draft, found := b.store.GetDraft(id)
	if !found {
		return b.sendText(ctx, msg.Chat.ID, "Черновик не найден.")
	}
	draft.Caption = value
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return b.sendText(ctx, msg.Chat.ID, "Подпись обновлена для "+draft.ID)
}

func (b *Bot) handleSetSource(ctx context.Context, msg Message) error {
	id, value, ok := splitCommandArg(msg.Text)
	if !ok {
		return b.sendText(ctx, msg.Chat.ID, "Формат: /source <draft_id> <текст или ссылка>")
	}
	draft, found := b.store.GetDraft(id)
	if !found {
		return b.sendText(ctx, msg.Chat.ID, "Черновик не найден.")
	}
	draft.SourceLabel = value
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return b.sendText(ctx, msg.Chat.ID, "Источник обновлен для "+draft.ID)
}

func (b *Bot) handlePublish(ctx context.Context, msg Message) error {
	parts := strings.Fields(msg.Text)
	if len(parts) != 2 {
		return b.sendText(ctx, msg.Chat.ID, "Формат: /publish <draft_id>")
	}
	if err := b.publishDraftByID(ctx, parts[1]); err != nil {
		return b.sendText(ctx, msg.Chat.ID, "Публикация не удалась: "+err.Error())
	}
	return b.sendText(ctx, msg.Chat.ID, "Опубликовано в канал: "+parts[1])
}

func (b *Bot) publishDraftByID(ctx context.Context, draftID string) error {
	draft, found := b.store.GetDraft(draftID)
	if !found {
		return fmt.Errorf("черновик не найден")
	}
	if draft.Status == DraftPublished {
		return fmt.Errorf("этот черновик уже опубликован")
	}

	caption := buildCaption(draft)
	if err := b.sendMediaToChannel(ctx, draft, caption); err != nil {
		draft.Status = DraftFailed
		draft.ErrorMessage = err.Error()
		_ = b.store.SaveDraft(draft)
		return err
	}

	now := time.Now().UTC()
	draft.Status = DraftPublished
	draft.PublishedAt = &now
	draft.ErrorMessage = ""
	if err := b.removeDraftMedia(&draft); err != nil {
		b.logger.Printf("cleanup published draft media %s: %v", draft.ID, err)
	}
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return nil
}

func (b *Bot) discardDraftByID(draftID string) error {
	draft, found := b.store.GetDraft(draftID)
	if !found {
		return fmt.Errorf("черновик не найден")
	}
	if err := b.removeDraftMedia(&draft); err != nil {
		return err
	}
	draft.Status = DraftFailed
	draft.ErrorMessage = "discarded by user"
	return b.store.SaveDraft(draft)
}

func (b *Bot) handleDelete(ctx context.Context, msg Message) error {
	parts := strings.Fields(msg.Text)
	if len(parts) != 2 {
		return b.sendText(ctx, msg.Chat.ID, "Формат: /delete <draft_id>")
	}
	draft, found := b.store.GetDraft(parts[1])
	if !found {
		return b.sendText(ctx, msg.Chat.ID, "Черновик не найден.")
	}
	if err := os.Remove(draft.LocalPath); err != nil && !os.IsNotExist(err) {
		return b.sendText(ctx, msg.Chat.ID, "Не удалось удалить файл: "+err.Error())
	}
	draft.LocalPath = ""
	draft.Status = DraftFailed
	draft.ErrorMessage = "deleted by user"
	if err := b.store.SaveDraft(draft); err != nil {
		return err
	}
	return b.sendText(ctx, msg.Chat.ID, "Файл удален, черновик помечен как удаленный: "+draft.ID)
}

func (b *Bot) sendHelp(ctx context.Context, chatID int64) error {
	help := strings.Join([]string{
		"Что умеет бот:",
		"- Принимает ссылку на YouTube Shorts / Instagram Reels / другие URL, которые понимает yt-dlp.",
		"- Принимает фото, видео и документы с image/video mime-type, в том числе пересланные сообщения из Telegram.",
		"- Хранит каждый импорт как черновик и публикует его в канал по команде.",
		"",
		"Команды:",
		"/list",
		"/caption <draft_id> <текст>",
		"/source <draft_id> <текст или ссылка>",
		"/publish <draft_id>",
		"/delete <draft_id>",
		"",
		"Подсказка: просто пришли ссылку или медиа, потом при необходимости обнови подпись и источник.",
	}, "\n")
	return b.sendText(ctx, chatID, help)
}

func (b *Bot) sendDraftList(ctx context.Context, chatID, ownerUserID int64) error {
	drafts := b.store.ListDrafts(ownerUserID)
	if len(drafts) == 0 {
		return b.sendText(ctx, chatID, "Черновиков пока нет.")
	}
	limit := 10
	if len(drafts) < limit {
		limit = len(drafts)
	}
	lines := []string{"Последние черновики:"}
	for _, d := range drafts[:limit] {
		lines = append(lines, fmt.Sprintf("%s | %s | %s", d.ID, d.Status, describeDraftShort(d)))
	}
	return b.sendText(ctx, chatID, strings.Join(lines, "\n"))
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	form := url.Values{}
	form.Set("timeout", "30")
	if offset > 0 {
		form.Set("offset", fmt.Sprintf("%d", offset))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL+"/getUpdates", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram returned ok=false")
	}
	return payload.Result, nil
}

func (b *Bot) sendText(ctx context.Context, chatID int64, text string) error {
	return b.sendTextWithKeyboard(ctx, chatID, text, nil)
}

func (b *Bot) sendTextWithKeyboard(ctx context.Context, chatID int64, text string, keyboard *inlineKeyboardMarkup) error {
	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", chatID))
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")
	if keyboard != nil {
		payload, err := json.Marshal(keyboard)
		if err != nil {
			return err
		}
		form.Set("reply_markup", string(payload))
	}
	return b.postForm(ctx, "/sendMessage", form)
}

func (b *Bot) answerCallbackQuery(ctx context.Context, callbackID, text string) error {
	form := url.Values{}
	form.Set("callback_query_id", callbackID)
	if text != "" {
		form.Set("text", text)
	}
	return b.postForm(ctx, "/answerCallbackQuery", form)
}

func (b *Bot) deleteMessage(ctx context.Context, chatID, messageID int64) error {
	form := url.Values{}
	form.Set("chat_id", fmt.Sprintf("%d", chatID))
	form.Set("message_id", fmt.Sprintf("%d", messageID))
	return b.postForm(ctx, "/deleteMessage", form)
}

func (b *Bot) postForm(ctx context.Context, method string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var payload APIResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if !payload.OK {
		return fmt.Errorf("telegram api error: %s", payload.Description)
	}
	return nil
}

func (b *Bot) downloadTelegramFile(ctx context.Context, fileID, originalName string) (string, error) {
	form := url.Values{}
	form.Set("file_id", fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL+"/getFile", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload GetFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.OK {
		return "", fmt.Errorf("telegram getFile failed")
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.cfg.BotToken, payload.Result.FilePath)
	fileResp, err := b.client.Get(fileURL)
	if err != nil {
		return "", err
	}
	defer fileResp.Body.Close()

	targetName := filepath.Base(originalName)
	if targetName == "" || targetName == "." {
		targetName = filepath.Base(payload.Result.FilePath)
	}
	if targetName == "" {
		targetName = "telegram_media"
	}
	targetPath := b.store.MediaPath(fmt.Sprintf("%d_%s", time.Now().UnixNano(), targetName))
	dst, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, fileResp.Body); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (b *Bot) sendMediaToChannel(ctx context.Context, draft Draft, caption string) error {
	file, err := os.Open(draft.LocalPath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("chat_id", b.cfg.ChannelID); err != nil {
		return err
	}
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return err
		}
	}

	fieldName := "document"
	method := "/sendDocument"
	switch draft.MediaKind {
	case MediaPhoto:
		fieldName = "photo"
		method = "/sendPhoto"
	case MediaVideo:
		fieldName = "video"
		method = "/sendVideo"
	}

	part, err := writer.CreateFormFile(fieldName, filepath.Base(draft.LocalPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL+method, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var payload APIResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode publish response: %w", err)
	}
	if !payload.OK {
		return fmt.Errorf("telegram publish error: %s", payload.Description)
	}
	return nil
}

func (b *Bot) moveIntoStore(tempPath, originalName string) (string, error) {
	targetName := originalName
	if targetName == "" {
		targetName = filepath.Base(tempPath)
	}
	targetPath := b.store.MediaPath(fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(targetName)))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func describeDraftShort(d Draft) string {
	parts := []string{string(d.MediaKind)}
	if d.Caption != "" {
		parts = append(parts, "caption")
	}
	if d.SourceLabel != "" || d.SourceURL != "" {
		parts = append(parts, "source")
	}
	return strings.Join(parts, ", ")
}

func formatDraftReadyMessage(d Draft) string {
	lines := []string{
		fmt.Sprintf("Черновик готов: %s", d.ID),
		fmt.Sprintf("Тип: %s", d.MediaKind),
		fmt.Sprintf("Файл: %s", filepath.Base(d.LocalPath)),
	}
	if d.OriginSummary != "" {
		lines = append(lines, "Источник импорта: "+d.OriginSummary)
	}
	if d.SourceURL != "" {
		lines = append(lines, "Исходная ссылка: "+d.SourceURL)
	}
	lines = append(lines, "", "Команды:")
	lines = append(lines, fmt.Sprintf("/caption %s Текст поста", d.ID))
	lines = append(lines, fmt.Sprintf("/source %s https://example.com/source", d.ID))
	lines = append(lines, fmt.Sprintf("/publish %s", d.ID))
	return strings.Join(lines, "\n")
}

func draftKeyboard(d Draft) *inlineKeyboardMarkup {
	return &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineKeyboardButton{
			{
				{
					Text:         "Publish",
					CallbackData: "publish:" + d.ID,
				},
				{
					Text:         "Discard",
					CallbackData: "discard:" + d.ID,
				},
			},
		},
	}
}

func buildCaption(d Draft) string {
	parts := make([]string, 0, 2)
	if d.Caption != "" {
		parts = append(parts, strings.TrimSpace(d.Caption))
	}
	source := strings.TrimSpace(d.SourceLabel)
	if source != "" {
		parts = append(parts, "Источник: "+source)
	}
	return strings.Join(parts, "\n\n")
}

func (b *Bot) cleanupDraftMedia() error {
	now := time.Now().UTC()
	drafts := b.store.ListAllDrafts()
	for _, draft := range drafts {
		switch {
		case draft.Status == DraftPublished && draft.LocalPath != "":
			if err := b.removeDraftMedia(&draft); err != nil {
				return err
			}
			if err := b.store.SaveDraft(draft); err != nil {
				return err
			}
		case draft.Status == DraftReady && draft.LocalPath != "" && now.Sub(draft.CreatedAt) > 72*time.Hour:
			if err := b.removeDraftMedia(&draft); err != nil {
				return err
			}
			draft.Status = DraftFailed
			draft.ErrorMessage = "expired after 72h without publish"
			if err := b.store.SaveDraft(draft); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Bot) removeDraftMedia(draft *Draft) error {
	if draft.LocalPath == "" {
		return nil
	}
	if err := os.Remove(draft.LocalPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	draft.LocalPath = ""
	return nil
}

func splitCommandArg(text string) (id, value string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return "", "", false
	}
	id = fields[1]
	prefix := strings.Join(fields[:2], " ")
	value = strings.TrimSpace(strings.TrimPrefix(text, prefix))
	return id, value, value != ""
}

func extractMedia(msg Message) (fileID, originalName string, kind MediaKind) {
	if len(msg.Photo) > 0 {
		sort.Slice(msg.Photo, func(i, j int) bool {
			return msg.Photo[i].FileSize > msg.Photo[j].FileSize
		})
		return msg.Photo[0].FileID, msg.Photo[0].FileUniqueID + ".jpg", MediaPhoto
	}
	if msg.Video != nil {
		name := msg.Video.FileName
		if name == "" {
			name = msg.Video.FileUniqueID + extensionFromMime(msg.Video.MimeType, ".mp4")
		}
		return msg.Video.FileID, name, MediaVideo
	}
	if msg.Document != nil {
		kind := DetectMediaKind(msg.Document.FileName, msg.Document.MimeType)
		if kind == "" {
			return "", "", ""
		}
		name := msg.Document.FileName
		if name == "" {
			name = msg.Document.FileUniqueID + extensionFromMime(msg.Document.MimeType, "")
		}
		return msg.Document.FileID, name, kind
	}
	return "", "", ""
}

func hasMedia(msg Message) bool {
	if len(msg.Photo) > 0 || msg.Video != nil {
		return true
	}
	if msg.Document != nil {
		return DetectMediaKind(msg.Document.FileName, msg.Document.MimeType) != ""
	}
	return false
}

func describeOrigin(msg Message) string {
	if msg.ForwardOrigin != nil {
		return "forwarded telegram message"
	}
	return "uploaded telegram media"
}

func DetectMediaKind(fileName, mimeType string) MediaKind {
	lowerName := strings.ToLower(fileName)
	lowerMime := strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(lowerMime, "image/"):
		return MediaPhoto
	case strings.HasPrefix(lowerMime, "video/"):
		return MediaVideo
	case strings.HasSuffix(lowerName, ".jpg"), strings.HasSuffix(lowerName, ".jpeg"), strings.HasSuffix(lowerName, ".png"), strings.HasSuffix(lowerName, ".webp"), strings.HasSuffix(lowerName, ".gif"):
		return MediaPhoto
	case strings.HasSuffix(lowerName, ".mp4"), strings.HasSuffix(lowerName, ".mov"), strings.HasSuffix(lowerName, ".m4v"), strings.HasSuffix(lowerName, ".webm"):
		return MediaVideo
	default:
		return ""
	}
}

func extensionFromMime(mimeType, fallback string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	default:
		return fallback
	}
}

func firstURL(values ...string) string {
	for _, value := range values {
		for _, token := range strings.Fields(value) {
			if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
				return strings.TrimSpace(token)
			}
		}
	}
	return ""
}
