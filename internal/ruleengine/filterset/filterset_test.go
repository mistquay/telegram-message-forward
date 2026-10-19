package filterset

import (
	"context"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	"testing"
)

type batchRepo struct{ calls int }

func (r *batchRepo) GetByID(context.Context, int64) (*domainfilter.Filter, error) {
	panic("should batch")
}
func (r *batchRepo) GetByIDs(context.Context, []int64) ([]*domainfilter.Filter, error) {
	r.calls++
	return []*domainfilter.Filter{{ID: 1, Name: "shared", Revision: r.calls, Conditions: []domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"go"}}}}}}, nil
}
func TestResolveUsesOneBatchAndFreezesVersion(t *testing.T) {
	r := &batchRepo{}
	snapshot, err := Resolve(context.Background(), r, []int64{1, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		passed, trace, _, err := snapshot.Evaluate(context.Background(), &domainmessage.NormalizedMessage{Text: "go"}, []int64{1}, nil)
		if err != nil || !passed || trace[0].Revision != 1 {
			t.Fatalf("%v %v %v", passed, trace, err)
		}
	}
	if r.calls != 1 {
		t.Fatal("单次求值必须固定共享配置快照")
	}
}
