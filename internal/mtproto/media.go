package mtproto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// DownloadMedia downloads the media from a message to the specified output directory.
// Returns the saved file path.
func DownloadMedia(ctx context.Context, api *tg.Client, msg *tg.Message, outDir string) (string, error) {
	if msg.Media == nil {
		return "", fmt.Errorf("message has no media")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	switch m := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		if m.Photo == nil {
			return "", fmt.Errorf("photo is nil")
		}
		photo, ok := m.Photo.(*tg.Photo)
		if !ok {
			return "", fmt.Errorf("unexpected photo type: %T", m.Photo)
		}
		loc := &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     "",
		}
		name := fmt.Sprintf("photo_%d.jpg", msg.ID)
		outPath := filepath.Join(outDir, name)
		_, err := downloader.NewDownloader().Download(api, loc).ToPath(ctx, outPath)
		return outPath, err

	case *tg.MessageMediaDocument:
		doc, ok := m.Document.(*tg.Document)
		if !ok {
			return "", fmt.Errorf("unexpected document type: %T", m.Document)
		}
		loc := &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		}
		name := documentFilename(doc)
		outPath := filepath.Join(outDir, name)
		_, err := downloader.NewDownloader().Download(api, loc).ToPath(ctx, outPath)
		return outPath, err

	default:
		return "", fmt.Errorf("unsupported media type: %T", msg.Media)
	}
}

// UploadFile uploads a local file and returns an InputFileClass suitable
// for sending as a message attachment.
func UploadFile(ctx context.Context, api *tg.Client, filePath string) (tg.InputFileClass, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	return uploader.NewUploader(api).FromPath(ctx, filePath)
}

// UploadAndSendMedia uploads a file and sends it as a message.
func UploadAndSendMedia(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, filePath, caption string) (int64, error) {
	inputFile, err := UploadFile(ctx, api, filePath)
	if err != nil {
		return 0, err
	}

	// Determine media type from extension
	ext := filepath.Ext(filePath)
	attrs := []tg.DocumentAttributeClass{}
	var inputMedia tg.InputMediaClass

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		inputMedia = &tg.InputMediaUploadedPhoto{
			File: inputFile,
		}
	default:
		attrs = append(attrs, &tg.DocumentAttributeFilename{
			FileName: filepath.Base(filePath),
		})
		inputMedia = &tg.InputMediaUploadedDocument{
			File:       inputFile,
			MimeType:   mimeFromExt(ext),
			Attributes: attrs,
		}
	}

	rnd := int64(0)
	resp, err := api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    inputMedia,
		Message:  caption,
		RandomID: rnd,
	})
	if err != nil {
		return 0, fmt.Errorf("send media: %w", err)
	}
	return extractMessageID(resp)
}

func documentFilename(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
			if fn.FileName != "" {
				return fn.FileName
			}
		}
	}
	return fmt.Sprintf("document_%d", doc.ID)
}

func mimeFromExt(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
