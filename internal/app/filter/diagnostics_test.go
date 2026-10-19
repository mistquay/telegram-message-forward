package filter

import (
	"context"
	"testing"
	"time"

	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
)

type previewFilterRepo struct{ domainfilter.Repository }

func (previewFilterRepo) GetByID(context.Context, int64) (*domainfilter.Filter, error) {
	return &domainfilter.Filter{ID: 1, Revision: 3, Conditions: []domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"go"}}}}}, nil
}
func (previewFilterRepo) Create(context.Context, *domainfilter.Filter) error {
	panic("preview must not save")
}

func TestPreviewComparesDraftWithoutSaving(t *testing.T) {
	s := NewService(previewFilterRepo{})
	r, err := s.Preview(context.Background(), PreviewInput{CompareID: 1, Conditions: []domainflow.ConditionConfig{{Type: "keyword_excludes", Config: map[string]any{"keywords": []string{"spam"}}}}, Message: &domainmessage.NormalizedMessage{Text: "go spam"}})
	if err != nil || r.Changed != 1 || r.Blocked != 1 || r.PreviousRevision != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Items[0].Previous.Status != "passed" || r.Items[0].Result.Status != "blocked" {
		t.Fatal("新旧判断结果不正确")
	}
}

func TestPreviewRequiresBoundedHistoryRange(t *testing.T) {
	s := NewService(previewFilterRepo{}).UseDiagnostics(DiagnosticsDeps{Queries: historyQuery{}})
	start := time.Now().Add(-40 * 24 * time.Hour)
	end := time.Now()
	_, err := s.Preview(context.Background(), PreviewInput{SourceID: 1, From: &start, To: &end, Conditions: []domainflow.ConditionConfig{{Type: "has_media"}}})
	if err == nil {
		t.Fatal("不能扫描无界历史")
	}
}

type historyQuery struct{ domainmessage.QueryRepository }

func (historyQuery) List(context.Context, domainmessage.Query) ([]domainmessage.ListItem, bool, error) {
	panic("unbounded history must be rejected before query")
}
