package chatarchive

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	domainarchive "telegram-message-forward/internal/domain/chatarchive"
)

type packageMediaEntry struct {
	MessageID int64  `json:"message_id"`
	Index     int    `json:"index"`
	Type      string `json:"type"`
	FileName  string `json:"file_name,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

type packageManifest struct {
	Version          int                 `json:"version"`
	ArchiveID        int64               `json:"archive_id"`
	PeerType         string              `json:"peer_type"`
	PeerID           int64               `json:"peer_id"`
	PeerName         string              `json:"peer_name,omitempty"`
	GeneratedAt      time.Time           `json:"generated_at"`
	MessageFormat    string              `json:"message_format"`
	FromDate         *time.Time          `json:"from_date,omitempty"`
	ToDate           *time.Time          `json:"to_date,omitempty"`
	FromMessageID    int64               `json:"from_message_id,omitempty"`
	ToMessageID      int64               `json:"to_message_id,omitempty"`
	IncludeService   bool                `json:"include_service"`
	MediaTotal       int                 `json:"media_total"`
	MediaIncluded    int                 `json:"media_included"`
	MediaUnavailable int                 `json:"media_unavailable"`
	Media            []packageMediaEntry `json:"media"`
}

// RenderPackage 流式写出正文文件、媒体文件和 manifest.json。
func (s *Service) RenderPackage(ctx context.Context, format Format, q domainarchive.ExportQuery, w io.Writer) error {
	if err := ValidateExportQuery(q); err != nil {
		return err
	}
	archive, err := s.archives.GetByID(ctx, q.ArchiveID)
	if err != nil {
		return err
	}

	zw := zip.NewWriter(w)
	messages, err := zw.CreateHeader(&zip.FileHeader{Name: "messages." + string(format), Method: zip.Deflate})
	if err != nil {
		return err
	}
	if err := s.Render(ctx, format, q, messages); err != nil {
		_ = zw.Close()
		return err
	}

	manifest := packageManifest{
		Version: 1, ArchiveID: archive.ID, PeerType: string(archive.PeerType),
		PeerID: archive.PeerID, PeerName: archive.PeerName,
		GeneratedAt: s.clk.Now().UTC(), MessageFormat: string(format),
		FromDate: q.From, ToDate: q.To, IncludeService: q.IncludeService,
		FromMessageID: q.FromMessageID, ToMessageID: q.ToMessageID,
		Media: make([]packageMediaEntry, 0),
	}
	err = s.streamMessages(ctx, q, func(message *domainarchive.Message) error {
		for index, media := range message.Media {
			manifest.MediaTotal++
			entry := packageMediaEntry{
				MessageID: message.MessageID, Index: index, Type: media.Type,
				FileName: media.FileName, MimeType: media.MimeType, Size: media.Size,
			}
			if !media.Downloaded || media.StorageKey == "" {
				entry.Status = "not_downloaded"
				entry.Reason = "归档中只有媒体元信息"
				manifest.MediaUnavailable++
				manifest.Media = append(manifest.Media, entry)
				continue
			}
			if s.media == nil {
				entry.Status = "missing"
				entry.Reason = "媒体存储未配置"
				manifest.MediaUnavailable++
				manifest.Media = append(manifest.Media, entry)
				continue
			}
			rc, openErr := s.media.Open(ctx, media.StorageKey)
			if openErr != nil {
				entry.Status = "missing"
				entry.Reason = "存储中不存在或无法读取"
				s.log.Warn("打包归档媒体时无法读取文件", "archive", q.ArchiveID,
					"message_id", message.MessageID, "key", media.StorageKey, "err", openErr)
				manifest.MediaUnavailable++
				manifest.Media = append(manifest.Media, entry)
				continue
			}
			firstChunk := make([]byte, 32*1024)
			readCount, firstReadErr := rc.Read(firstChunk)
			if firstReadErr != nil && firstReadErr != io.EOF {
				rc.Close()
				entry.Status = "missing"
				entry.Reason = "存储中不存在或无法读取"
				s.log.Warn("打包归档媒体时首读失败", "archive", q.ArchiveID,
					"message_id", message.MessageID, "key", media.StorageKey, "err", firstReadErr)
				manifest.MediaUnavailable++
				manifest.Media = append(manifest.Media, entry)
				continue
			}
			entry.Path = packageMediaPath(message.MessageID, index, media)
			file, createErr := zw.CreateHeader(&zip.FileHeader{Name: entry.Path, Method: zip.Store})
			if createErr != nil {
				rc.Close()
				return createErr
			}
			if readCount > 0 {
				if _, writeErr := file.Write(firstChunk[:readCount]); writeErr != nil {
					rc.Close()
					return writeErr
				}
			}
			_, copyErr := io.Copy(file, rc)
			closeErr := rc.Close()
			if copyErr != nil {
				return fmt.Errorf("读取媒体 %q 失败: %w", media.StorageKey, copyErr)
			}
			if closeErr != nil {
				return closeErr
			}
			entry.Status = "included"
			manifest.MediaIncluded++
			manifest.Media = append(manifest.Media, entry)
		}
		return nil
	})
	if err != nil {
		_ = zw.Close()
		return err
	}

	manifestWriter, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate})
	if err != nil {
		_ = zw.Close()
		return err
	}
	encoder := json.NewEncoder(manifestWriter)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func packageMediaPath(messageID int64, index int, media domainarchive.Media) string {
	rawName := path.Base(strings.ReplaceAll(media.FileName, "\\", "/"))
	name := sanitizeFileName(rawName)
	if strings.TrimSpace(rawName) == "" {
		ext := path.Ext(media.StorageKey)
		name = "media-" + strconv.Itoa(index) + ext
	}
	return fmt.Sprintf("media/%d/%d-%s", messageID, index, name)
}
