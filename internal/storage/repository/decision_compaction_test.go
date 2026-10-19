package repository

import (
	"encoding/json"
	"strings"
	"telegram-message-forward/internal/domain/decision"
	"testing"
)

func TestOversizedDecisionKeepsOutcomesAndExactTotals(t *testing.T) {
	evaluation := decision.Evaluation{Status: "blocked", FilterTotals: []decision.FilterCount{{ID: 1, Blocked: 100}}, Flows: []decision.Flow{{ID: 1, Status: "blocked"}}}
	for i := 0; i < 100; i++ {
		evaluation.Flows[0].Nodes = append(evaluation.Flows[0].Nodes, decision.Node{NodeID: int64(i + 1), Status: "blocked", ConfigSnapshot: json.RawMessage(`{"large":"` + strings.Repeat("x", 20000) + `"}`), Filters: []decision.Filter{{ID: 1, Name: "filter", Result: decision.Condition{Type: "all", Status: "blocked", Children: []decision.Condition{{Type: "keyword_excludes", Status: "blocked", Evidence: "spam"}}}}}})
	}
	compacted := compactEvaluation(evaluation)
	data, err := json.Marshal(compacted)
	if err != nil || len(data) > 1024*1024 {
		t.Fatalf("bounded trace size=%d err=%v", len(data), err)
	}
	if !compacted.Truncated || compacted.Status != "blocked" || compacted.FilterTotals[0].Blocked != 100 {
		t.Fatal("压缩详情不能改变结果与统计")
	}
	if len(evaluation.Flows[0].Nodes[0].Filters[0].Result.Children) == 0 {
		t.Fatal("不能修改原始求值结果")
	}
}
