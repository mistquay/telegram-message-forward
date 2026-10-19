// Package filter 提供可复用过滤器的管理应用服务。
package filter

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	"telegram-message-forward/internal/ruleengine/condition"
)

// Service 是过滤器应用服务。
type Service struct {
	repo domainfilter.Repository
	deps DiagnosticsDeps
}

func NewService(repo domainfilter.Repository) *Service {
	return &Service{repo: repo}
}

// Input 是过滤器创建/更新输入。
type Input struct {
	Revision    int
	Name        string
	Description string
	Conditions  []domainflow.ConditionConfig
}

func (s *Service) List(ctx context.Context) ([]*domainfilter.Filter, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*domainfilter.Filter, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, in Input) (*domainfilter.Filter, error) {
	f := fromInput(0, in)
	if err := s.validate(f); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Service) Update(ctx context.Context, id int64, in Input) (*domainfilter.Filter, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	f := fromInput(id, in)
	f.CreatedAt = existing.CreatedAt
	if in.Revision > 0 && in.Revision != existing.Revision {
		return nil, fmt.Errorf("%w，请刷新后重试", domainfilter.ErrConflict)
	}
	f.Revision = existing.Revision
	if err := s.validate(f); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	count, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w：%d 处（Flow/AI 整理），无法删除", domainfilter.ErrInUse, count)
	}
	return s.repo.Delete(ctx, id)
}

func fromInput(id int64, in Input) *domainfilter.Filter {
	return &domainfilter.Filter{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
		Conditions:  in.Conditions,
	}
}

func (s *Service) validate(f *domainfilter.Filter) error {
	if f.Name == "" {
		return &condition.ValidationError{Path: "name", Message: "过滤器名称不能为空"}
	}
	if utf8.RuneCountInString(f.Name) > 100 {
		return &condition.ValidationError{Path: "name", Message: "名称不能超过 100 个字符"}
	}
	if utf8.RuneCountInString(f.Description) > 2000 {
		return &condition.ValidationError{Path: "description", Message: "说明不能超过 2000 个字符"}
	}
	if len(f.Conditions) == 0 {
		return &condition.ValidationError{Path: "conditions", Message: "过滤器至少需要一个匹配条件"}
	}
	_, err := condition.Compile(f.Conditions)
	return err
}
