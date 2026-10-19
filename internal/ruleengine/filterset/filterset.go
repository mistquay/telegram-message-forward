// Package filterset resolves shared filters once per evaluation and keeps immutable programs.
package filterset

import (
	"context"
	"fmt"

	"telegram-message-forward/internal/domain/decision"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	"telegram-message-forward/internal/ruleengine/condition"
)

type Resolver interface {
	GetByID(context.Context, int64) (*domainfilter.Filter, error)
}

const MaxReferences = 32

type batchResolver interface {
	GetByIDs(context.Context, []int64) ([]*domainfilter.Filter, error)
}

type Entry struct {
	ID       int64
	Name     string
	Revision int
	Program  *condition.Program
}

type Snapshot map[int64]Entry

type ResolutionError struct {
	ID  int64
	Err error
}

func (e *ResolutionError) Error() string {
	return fmt.Sprintf("共享过滤器 id=%d 解析失败: %v", e.ID, e.Err)
}
func (e *ResolutionError) Unwrap() error { return e.Err }

func Resolve(ctx context.Context, resolver Resolver, ids []int64) (Snapshot, error) {
	out := Snapshot{}
	if len(ids) == 0 {
		return out, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("共享过滤器解析器未配置")
	}
	seen := map[int64]bool{}
	unique := []int64{}
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("共享过滤器 ID 无效")
		}
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	var filters []*domainfilter.Filter
	if batch, ok := resolver.(batchResolver); ok {
		var err error
		filters, err = batch.GetByIDs(ctx, unique)
		if err != nil {
			return nil, fmt.Errorf("批量加载共享过滤器失败: %w", err)
		}
	} else {
		for _, id := range unique {
			f, err := resolver.GetByID(ctx, id)
			if err != nil {
				return nil, &ResolutionError{ID: id, Err: err}
			}
			filters = append(filters, f)
		}
	}
	for _, f := range filters {
		if f == nil || len(f.Conditions) == 0 {
			return nil, fmt.Errorf("共享过滤器不存在或条件为空")
		}
		program, err := condition.Compile(f.Conditions)
		if err != nil {
			return nil, &ResolutionError{ID: f.ID, Err: err}
		}
		out[f.ID] = Entry{ID: f.ID, Name: f.Name, Revision: f.Revision, Program: program}
	}
	for _, id := range unique {
		if _, ok := out[id]; !ok {
			return nil, &ResolutionError{ID: id, Err: fmt.Errorf("过滤器不存在")}
		}
	}
	return out, nil
}

func (s Snapshot) Evaluate(ctx context.Context, msg *domainmessage.NormalizedMessage, ids []int64, inline []domainflow.ConditionConfig) (bool, []decision.Filter, *decision.Condition, error) {
	if len(ids) > MaxReferences {
		return false, nil, nil, fmt.Errorf("单个节点或 AI Profile 不能引用超过 %d 个共享过滤器", MaxReferences)
	}
	if len(ids) == 0 {
		p, err := condition.Compile(inline)
		if err != nil {
			return false, nil, nil, err
		}
		r, err := p.Evaluate(ctx, msg)
		return r.Status == "passed", nil, &r, err
	}
	out := []decision.Filter{}
	blocked := false
	for _, id := range ids {
		entry, ok := s[id]
		if !ok {
			return false, out, nil, fmt.Errorf("共享过滤器快照缺失 id=%d", id)
		}
		r := decision.Condition{Type: "all", Status: "skipped", Reason: "short_circuit"}
		var err error
		if !blocked {
			r, err = entry.Program.Evaluate(ctx, msg)
		}
		out = append(out, decision.Filter{ID: id, Name: entry.Name, Revision: entry.Revision, Hash: entry.Program.Hash, Result: r})
		if err != nil {
			return false, out, nil, err
		}
		blocked = blocked || r.Status != "passed"
	}
	return !blocked, out, nil, nil
}
