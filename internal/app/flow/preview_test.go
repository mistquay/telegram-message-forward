package flow

import (
	"context"
	"strings"
	"testing"

	domainflow "telegram-message-forward/internal/domain/flow"
	domainsink "telegram-message-forward/internal/domain/sink"
	"telegram-message-forward/internal/flowengine"
)

func TestPreviewReturnsEachBranchesProcessedText(t *testing.T) {
	src, a, b := int64(1), int64(10), int64(20)
	in := Input{Name: "preview", Enabled: true, Nodes: []domainflow.Node{
		{ID: 1, Type: domainflow.NodeTypeSource, RefID: &src},
		{ID: 2, Type: domainflow.NodeTypeProcessor, Config: domainflow.NodeConfig{Processors: []domainflow.ProcessorConfig{{Type: "append_source", Config: map[string]any{"text": " processed"}}}}},
		{ID: 3, Type: domainflow.NodeTypeTarget, RefID: &a},
		{ID: 4, Type: domainflow.NodeTypeTarget, RefID: &b},
	}, Edges: []domainflow.Edge{{FromNodeID: 1, ToNodeID: 2}, {FromNodeID: 2, ToNodeID: 3}, {FromNodeID: 1, ToNodeID: 4}}}
	s := NewService(nil, flowengine.NewEngine())
	r, err := s.Preview(context.Background(), PreviewInput{Flow: in, Message: PreviewMessage{SourceID: 1, Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Evaluation.Flows[0].Version, "draft:") {
		t.Fatal("草稿必须显示指纹，不能冒充已保存版本或零时间")
	}
	texts := map[int64]string{}
	for _, target := range r.Targets {
		texts[target.SinkID] = target.Text
	}
	if texts[a] != "original processed" || texts[b] != "original" {
		t.Fatalf("每条目标必须使用自己的分支快照: %+v", texts)
	}
}

type metadataSink struct{ domainsink.Repository }

func (metadataSink) GetByID(context.Context, int64) (*domainsink.Sink, error) {
	panic("read-only preview must not load credentials")
}
func (metadataSink) GetRoutingInfo(context.Context, int64) (*domainsink.Sink, error) {
	return &domainsink.Sink{Enabled: true, Capabilities: domainsink.Capabilities{SupportsText: true}}, nil
}

func TestPreviewOnlyReadsRoutingMetadata(t *testing.T) {
	source, sink := int64(1), int64(2)
	s := NewService(nil, flowengine.NewEngine(), ValidatorDeps{Sinks: metadataSink{}})
	r, err := s.Preview(context.Background(), PreviewInput{Flow: Input{Name: "safe read", Nodes: []domainflow.Node{{ID: 1, Type: domainflow.NodeTypeSource, RefID: &source}, {ID: 2, Type: domainflow.NodeTypeTarget, RefID: &sink}}, Edges: []domainflow.Edge{{FromNodeID: 1, ToNodeID: 2}}}, Message: PreviewMessage{SourceID: 1, Text: "hello"}})
	if err != nil || len(r.Targets) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}
