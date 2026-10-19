package condition

import (
	"context"
	"testing"
)

func TestInvalidConfigCannotSaveOrExecute(t *testing.T) {
	cases := []struct {
		typ string
		cfg map[string]any
	}{
		{"source", map[string]any{"source_ids": []any{"invalid"}}},
		{"source", map[string]any{"source_ids": []any{float64(7)}}},
		{"sender", map[string]any{"sender_ids": []any{"invalid"}}},
		{"sender", map[string]any{}},
		{"keyword_excludes", map[string]any{"keywords": []string{""}}},
		{"keyword_contains", map[string]any{"keywords": []string{"go"}, "case_sensitive": "false"}},
		{"message_length", map[string]any{"min": 10, "max": 2}},
		{"message_length", map[string]any{"min": -1}},
		{"message_type", map[string]any{"types": []string{"invalid"}}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			if err := ValidateConfig(tc.typ, tc.cfg); err == nil {
				t.Fatal("应拒绝非法配置")
			}
			c, _ := Get(tc.typ)
			if passed, err := c.Evaluate(context.Background(), msg("anything", "text"), tc.cfg); err == nil || passed {
				t.Fatalf("非法存量配置也不能放行 passed=%v err=%v", passed, err)
			}
		})
	}
}
