package aidigest

import (
	"context"
	"errors"
	"testing"

	domainaidigest "telegram-message-forward/internal/domain/aidigest"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
)

type failingFilterRepo struct{ domainfilter.Repository }

func (failingFilterRepo) GetByID(_ context.Context, id int64) (*domainfilter.Filter, error) {
	if id == 1 {
		return &domainfilter.Filter{ID: 1, Conditions: []domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"urgent"}}}}}, nil
	}
	return nil, errors.New("lookup failed")
}

func TestSharedFilterLookupFailureNeverFallsBack(t *testing.T) {
	for _, ids := range [][]int64{{2}, {1, 2}} {
		s := NewService(Deps{Filters: failingFilterRepo{}})
		p := &domainaidigest.Profile{FilterIDs: ids}
		_, included, err := s.filterMessages(context.Background(), p, 1, []*domainmessage.NormalizedMessage{{ID: 1, Text: "urgent spam"}})
		if err == nil || len(included) != 0 {
			t.Fatalf("共享过滤器读取失败不能放行 ids=%v included=%v err=%v", ids, included, err)
		}
	}
}

func TestAIHonorsSharedSourceCondition(t *testing.T) {
	s := NewService(Deps{})
	passed, err := s.matchesConditions(context.Background(), &domainmessage.NormalizedMessage{SourceID: 99}, []domainflow.ConditionConfig{{Type: "source", Config: map[string]any{"source_ids": []string{"7"}}}})
	if err != nil || passed {
		t.Fatalf("AI 不应跳过共享来源条件 passed=%v err=%v", passed, err)
	}
}
