package filter

import (
	"context"
	"fmt"
	"time"

	"telegram-message-forward/internal/domain/decision"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	"telegram-message-forward/internal/ruleengine/condition"
)

type DiagnosticsDeps struct {
	Messages  domainmessage.Repository
	Queries   domainmessage.QueryRepository
	Decisions decision.Repository
}

func (s *Service) ValidateConditions(configs []domainflow.ConditionConfig) error {
	_, err := condition.Compile(configs)
	return err
}

func (s *Service) UseDiagnostics(deps DiagnosticsDeps) *Service { s.deps = deps; return s }

func (s *Service) References(ctx context.Context, id int64) ([]domainfilter.Reference, error) {
	r, ok := s.repo.(domainfilter.DiagnosticsRepository)
	if !ok {
		return nil, fmt.Errorf("过滤器引用查询不可用")
	}
	return r.References(ctx, id)
}

func (s *Service) Versions(ctx context.Context, id int64, limit, offset int) ([]domainfilter.Version, int64, error) {
	r, ok := s.repo.(domainfilter.DiagnosticsRepository)
	if !ok {
		return nil, 0, fmt.Errorf("过滤器历史查询不可用")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	return r.Versions(ctx, id, limit, offset)
}

func (s *Service) Version(ctx context.Context, id int64, revision int) (*domainfilter.Version, error) {
	r, ok := s.repo.(domainfilter.DiagnosticsRepository)
	if !ok {
		return nil, fmt.Errorf("过滤器历史查询不可用")
	}
	return r.Version(ctx, id, revision)
}

func (s *Service) Stats(ctx context.Context) (map[int64]domainfilter.Stats, error) {
	r, ok := s.repo.(domainfilter.DiagnosticsRepository)
	if !ok {
		return map[int64]domainfilter.Stats{}, nil
	}
	return r.Stats(ctx, time.Now().Add(-24*time.Hour))
}

func (s *Service) Decisions(ctx context.Context, q decision.Query) ([]decision.Record, int64, error) {
	if s.deps.Decisions == nil {
		return nil, 0, fmt.Errorf("决策查询不可用")
	}
	return s.deps.Decisions.List(ctx, q)
}

type PreviewInput struct {
	Conditions []domainflow.ConditionConfig
	CompareID  int64
	Message    *domainmessage.NormalizedMessage
	MessageID  int64
	SourceID   int64
	From       *time.Time
	To         *time.Time
	BeforeID   int64
	Limit      int
}

type PreviewItem struct {
	MessageID   int64               `json:"message_id"`
	MessageText string              `json:"message_text"`
	Result      decision.Condition  `json:"result"`
	Previous    *decision.Condition `json:"previous,omitempty"`
	Changed     bool                `json:"changed"`
}

type PreviewResult struct {
	Items            []PreviewItem `json:"items"`
	Passed           int           `json:"passed"`
	Blocked          int           `json:"blocked"`
	Changed          int           `json:"changed"`
	NextCursor       int64         `json:"next_cursor,omitempty"`
	HasMore          bool          `json:"has_more"`
	ConfigHash       string        `json:"config_hash"`
	PreviousRevision int           `json:"previous_revision,omitempty"`
}

func (s *Service) Preview(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	if len(in.Conditions) == 0 {
		return nil, fmt.Errorf("至少配置一个条件")
	}
	program, err := condition.Compile(in.Conditions)
	if err != nil {
		return nil, err
	}
	result := &PreviewResult{Items: []PreviewItem{}, ConfigHash: program.Hash}
	var previous *condition.Program
	if in.CompareID > 0 {
		f, err := s.repo.GetByID(ctx, in.CompareID)
		if err != nil {
			return nil, err
		}
		previous, err = condition.Compile(f.Conditions)
		if err != nil {
			return nil, err
		}
		result.PreviousRevision = f.Revision
	}
	msgs := []*domainmessage.NormalizedMessage{}
	if in.MessageID > 0 {
		if s.deps.Messages == nil {
			return nil, fmt.Errorf("真实消息查询不可用")
		}
		m, err := s.deps.Messages.GetByID(ctx, in.MessageID)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	} else if in.SourceID > 0 {
		if s.deps.Queries == nil {
			return nil, fmt.Errorf("历史消息查询不可用")
		}
		if in.From == nil || in.To == nil || !in.From.Before(*in.To) || in.To.Sub(*in.From) > 31*24*time.Hour {
			return nil, fmt.Errorf("历史样本必须指定不超过 31 天的有效时间范围")
		}
		limit := in.Limit
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, hasMore, err := s.deps.Queries.List(ctx, domainmessage.Query{SourceID: in.SourceID, From: in.From, To: in.To, BeforeID: in.BeforeID, Limit: limit})
		if err != nil {
			return nil, err
		}
		result.HasMore = hasMore
		for _, row := range rows {
			msgs = append(msgs, row.Message)
		}
		if hasMore && len(rows) > 0 {
			result.NextCursor = rows[len(rows)-1].Message.ID
		}
	} else if in.Message != nil {
		msgs = append(msgs, in.Message)
	} else {
		return nil, fmt.Errorf("请选择样例、真实消息或历史样本")
	}
	for _, m := range msgs {
		if m.ReceivedAt.IsZero() {
			m.ReceivedAt = time.Now()
		}
		if m.MessageType == "" {
			m.MessageType = "text"
		}
		r, err := program.Evaluate(ctx, m)
		if err != nil {
			return nil, err
		}
		text := []rune(m.Text)
		if len(text) > 500 {
			text = text[:500]
		}
		item := PreviewItem{MessageID: m.ID, MessageText: string(text), Result: r}
		if previous != nil {
			old, err := previous.Evaluate(ctx, m)
			if err != nil {
				return nil, err
			}
			item.Previous = &old
			item.Changed = old.Status != r.Status
			if item.Changed {
				result.Changed++
			}
		}
		if r.Status == "passed" {
			result.Passed++
		} else {
			result.Blocked++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
