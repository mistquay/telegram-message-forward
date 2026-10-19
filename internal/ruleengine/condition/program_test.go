package condition

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
)

func TestProgramGroupsNegationAndSkippedEvidence(t *testing.T) {
	configs := []domainflow.ConditionConfig{{Type: "any", Children: []domainflow.ConditionConfig{
		{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"urgent"}}},
		{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"other"}}},
	}}, {Type: "keyword_contains", Negate: true, Config: map[string]any{"keywords": []string{"spam"}}}}
	p, err := Compile(configs)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Evaluate(context.Background(), msg("urgent", "text"))
	if err != nil || r.Status != "passed" {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Children[0].Children[1].Status != "skipped" {
		t.Fatal("OR 短路必须标记未执行")
	}
	r, err = p.Evaluate(context.Background(), msg("urgent spam", "text"))
	if err != nil || r.Status != "blocked" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestProgramTextScopeNormalizationAndImmutableCache(t *testing.T) {
	ctx := context.Background()
	configs := []domainflow.ConditionConfig{{Type: "keyword_excludes", Config: map[string]any{"keywords": []string{"spam"}, "fields": []string{"links"}}}}
	p, err := Compile(configs)
	if err != nil {
		t.Fatal(err)
	}
	configs[0].Config["keywords"] = []string{"other"}
	r, err := p.Evaluate(ctx, &domainmessage.NormalizedMessage{Text: "safe", Links: []domainmessage.Link{{URL: "https://spam.example/?token=secret"}}})
	if err != nil || r.Status != "blocked" {
		t.Fatalf("%+v %v", r, err)
	}
	if strings.Contains(r.Children[0].Evidence, "secret") {
		t.Fatal("不能记录 URL 查询参数")
	}
	p, err = Compile([]domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"spam"}, "normalize": "nfkc"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, _ = p.Evaluate(ctx, msg("ｓｐａｍ", "text"))
	if r.Status != "passed" {
		t.Fatal("NFKC 应按显式配置执行")
	}
	p, _ = Compile([]domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"spam"}}}})
	r, _ = p.Evaluate(ctx, msg("ｓｐａｍ", "text"))
	if r.Status != "blocked" {
		t.Fatal("旧配置不能自动规范化")
	}
}

func TestProgramTimeBasisAndBounds(t *testing.T) {
	sent := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	m := &domainmessage.NormalizedMessage{SentAt: &sent, ReceivedAt: sent.Add(4 * time.Hour)}
	cfg := []domainflow.ConditionConfig{{Type: "time_window", Config: map[string]any{"start": "12:00", "end": "12:00", "timezone": "UTC", "time_basis": "received"}}}
	p, err := Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := p.Evaluate(context.Background(), m)
	if r.Status != "passed" {
		t.Fatal("应按接收时间判断")
	}
	tooMany := make([]domainflow.ConditionConfig, 101)
	for i := range tooMany {
		tooMany[i] = domainflow.ConditionConfig{Type: "has_media"}
	}
	if _, err := Compile(tooMany); err == nil {
		t.Fatal("必须限制条件数")
	}
}

func BenchmarkCompiledKeywordEvaluation(b *testing.B) {
	p, _ := Compile([]domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"go"}}}})
	m := msg("go news", "text")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Evaluate(ctx, m)
	}
}

func TestMediaSelectorsKeepLegacyDocumentAndDistinguishFiles(t *testing.T) {
	m := &domainmessage.NormalizedMessage{MessageType: "document", Media: []domainmessage.Media{{Type: "document", MimeType: "video/mp4"}}}
	for _, kind := range []string{"message_type", "media_type"} {
		c, _ := Get(kind)
		for value, want := range map[string]bool{"document": true, "video": true, "file": false} {
			passed, err := c.Evaluate(context.Background(), m, map[string]any{"types": []string{value}})
			if err != nil || passed != want {
				t.Fatalf("%s %s passed=%v err=%v", kind, value, passed, err)
			}
		}
	}
}

func TestCustomSourceTypesRemainCompatible(t *testing.T) {
	m := &domainmessage.NormalizedMessage{MessageType: "custom_notice", SenderPeerType: "custom_sender", Media: []domainmessage.Media{{Type: "custom_attachment"}}}
	for typ, cfg := range map[string]map[string]any{"message_type": {"types": []string{"custom_notice"}}, "media_type": {"types": []string{"custom_attachment"}}, "sender": {"peer_types": []string{"custom_sender"}}} {
		c, _ := Get(typ)
		passed, err := c.Evaluate(context.Background(), m, cfg)
		if err != nil || !passed {
			t.Fatalf("旧 Source 自定义类型不应失效 %s %v %v", typ, passed, err)
		}
	}
}

func TestValidationErrorIncludesNestedFieldPath(t *testing.T) {
	_, err := Compile([]domainflow.ConditionConfig{{Type: "any", Children: []domainflow.ConditionConfig{{Type: "sender", Config: map[string]any{"sender_ids": []string{"invalid"}}}}}})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Path != "conditions.0.0.config.sender_ids" {
		t.Fatalf("无法定位配置字段: %v", err)
	}
}

func TestGetAndProgramHaveIdenticalScopeSemantics(t *testing.T) {
	c, _ := Get("keyword_excludes")
	passed, err := c.Evaluate(context.Background(), &domainmessage.NormalizedMessage{Text: "safe", Links: []domainmessage.Link{{URL: "https://spam.example"}}}, map[string]any{"keywords": []string{"spam"}, "fields": []string{"links"}})
	if err != nil || passed {
		t.Fatalf("公开条件接口必须与编译求值一致 passed=%v err=%v", passed, err)
	}
}
