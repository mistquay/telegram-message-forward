package chatarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	domainarchive "telegram-message-forward/internal/domain/chatarchive"
)

type packageTestStore map[string]string

func (s packageTestStore) PutFile(context.Context, string, string) (string, error) {
	return "", nil
}
func (s packageTestStore) PublicURL(context.Context, string) (string, error) { return "", nil }
func (s packageTestStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	value, ok := s[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(value)), nil
}
func (s packageTestStore) Cleanup(context.Context, time.Duration) (int, error) { return 0, nil }

func TestRenderPackageIncludesMessagesMediaAndManifest(t *testing.T) {
	items := sampleMessages()
	items[2].Media = []domainarchive.Media{
		{Type: "photo", FileName: "../相片.jpg", MimeType: "image/jpeg", Size: 3,
			Downloaded: true, StorageKey: "chat-archive/5/photo.jpg"},
		{Type: "document", FileName: "missing.pdf", Downloaded: true,
			StorageKey: "chat-archive/5/missing.pdf"},
		{Type: "video", FileName: "clip.mp4", Downloaded: false},
	}
	repo := &stubMessageRepo{all: items}
	svc := testService(repo, &domainarchive.Archive{
		ID: 5, PeerName: "阿达", PeerType: domainarchive.PeerUser, PeerID: 7,
	}).UseMediaStore(packageTestStore{"chat-archive/5/photo.jpg": "img"})

	var buf bytes.Buffer
	if err := svc.RenderPackage(context.Background(), FormatJSONL, domainarchive.ExportQuery{
		ArchiveID: 5, FromMessageID: 100, ToMessageID: 200,
	}, &buf); err != nil {
		t.Fatalf("RenderPackage: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("打开 ZIP: %v", err)
	}
	files := map[string]string{}
	for _, file := range zr.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = string(raw)
	}
	if !strings.Contains(files["messages.jsonl"], `"message_id":103`) {
		t.Fatalf("正文文件缺少消息: %s", files["messages.jsonl"])
	}
	if files["media/103/0-相片.jpg"] != "img" {
		t.Fatalf("媒体文件未打包或路径不安全: %+v", files)
	}
	manifest := files["manifest.json"]
	if !strings.Contains(manifest, `"media_included": 1`) ||
		!strings.Contains(manifest, `"media_unavailable": 2`) ||
		!strings.Contains(manifest, `"status": "missing"`) ||
		!strings.Contains(manifest, `"status": "not_downloaded"`) {
		t.Fatalf("manifest 统计不完整: %s", manifest)
	}
}

func TestValidateExportQueryRejectsMixedRanges(t *testing.T) {
	from := time.Now()
	err := ValidateExportQuery(domainarchive.ExportQuery{
		ArchiveID: 1, From: &from, FromMessageID: 10,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("混用时间和 ID 范围应拒绝: %v", err)
	}
}
