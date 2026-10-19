package flowengine

import (
	"context"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	"testing"
)

func TestTraceRecordsBlockedBranchesAndStopOnMatch(t *testing.T) {
	f := linearFlow(1, 10, 100, contains("go"), nil)
	f.Priority = 2
	f.StopOnMatch = true
	f.Nodes = append(f.Nodes, filterNode(4, contains("spam")))
	f.Edges = append(f.Edges, domainflow.Edge{FromNodeID: 1, ToNodeID: 4}, domainflow.Edge{FromNodeID: 4, ToNodeID: 3})
	low := linearFlow(2, 10, 200, contains("go"), nil)
	matches, result, err := NewEngine().EvaluateDetailed(context.Background(), msg("go"), []*domainflow.Flow{f, low})
	if err != nil || len(matches) != 1 || result.Status != "matched" {
		t.Fatalf("%+v %v", result, err)
	}
	if result.Flows[1].Reason != "stop_on_match" {
		t.Fatal("必须说明低优先级未执行原因")
	}
	found := false
	for _, n := range result.Flows[0].Nodes {
		if n.NodeID == 4 {
			found = n.Status == "blocked"
		}
	}
	if !found {
		t.Fatal("必须保留失败分支，不能只有最终目标")
	}
}

type traceFilterResolver struct{}

func (traceFilterResolver) GetByID(context.Context, int64) (*domainfilter.Filter, error) {
	return &domainfilter.Filter{ID: 1, Name: "shared", Revision: 3, Conditions: []domainflow.ConditionConfig{contains("go")}}, nil
}

func TestSkippedReferencesKeepVersionsForFilterDiagnostics(t *testing.T) {
	high := linearFlow(1, 10, 100, contains("go"), nil)
	high.Priority = 10
	high.StopOnMatch = true
	low := linearFlow(2, 10, 200, contains("go"), nil)
	low.Nodes[1].Config = domainflow.NodeConfig{FilterIDs: []int64{1}}
	_, trace, err := NewEngine(WithFilterResolver(traceFilterResolver{})).EvaluateDetailed(context.Background(), msg("go"), []*domainflow.Flow{high, low})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range trace.Flows[1].Nodes {
		if len(step.Filters) > 0 {
			f := step.Filters[0]
			found = f.ID == 1 && f.Revision == 3 && f.Result.Status == "skipped" && f.Result.Reason == "stop_on_match"
		}
	}
	if !found {
		t.Fatal("过滤器详情必须能关联被命中即停跳过的消息，不能伪装为没有收到消息")
	}
}
