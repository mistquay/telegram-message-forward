package telegram

import (
	"context"
	"github.com/gotd/td/tg"
	"telegram-message-forward/internal/ruleengine/condition"
	"testing"
)

func TestNormalizeMediaTypesMatchFilterOptions(t *testing.T) {
	voice := &tg.DocumentAttributeAudio{Voice: true}
	for _, tc := range []struct {
		name, mime, kind string
		attrs            []tg.DocumentAttributeClass
	}{
		{"video", "video/mp4", "video", []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{W: 1280, H: 720}}},
		{"audio", "audio/mpeg", "audio", []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{}}},
		{"voice", "audio/ogg", "voice", []tg.DocumentAttributeClass{voice}},
		{"file", "application/pdf", "document", nil},
		{"image", "image/png", "image", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := &tg.MessageMediaDocument{}
			media.SetDocument(&tg.Document{ID: 1, MimeType: tc.mime, Attributes: tc.attrs})
			m := &tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 1}}
			m.SetMedia(media)
			n := Normalize(1, m, tg.Entities{})
			if n.MessageType != tc.kind || n.Media[0].Type != tc.kind {
				t.Fatalf("%+v", n)
			}
			c, _ := condition.Get("media_type")
			passed, err := c.Evaluate(context.Background(), n, map[string]any{"types": []string{tc.kind}})
			if err != nil || !passed {
				t.Fatalf("过滤选项必须匹配真实类型 %v %v", passed, err)
			}
		})
	}
}
