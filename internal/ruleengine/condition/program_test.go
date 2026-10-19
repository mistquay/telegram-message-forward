package condition

import (
	"context"
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
