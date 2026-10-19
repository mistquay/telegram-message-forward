package flowengine

import (
	"context"
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
