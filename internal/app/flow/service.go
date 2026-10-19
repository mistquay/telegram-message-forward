// Package flow 提供 Flow 图编排应用服务。
package flow

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"telegram-message-forward/internal/domain/decision"
	domaindelivery "telegram-message-forward/internal/domain/delivery"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	domainsink "telegram-message-forward/internal/domain/sink"
	domainsource "telegram-message-forward/internal/domain/source"
	domaintemplate "telegram-message-forward/internal/domain/template"
	"telegram-message-forward/internal/flowengine"
	"telegram-message-forward/internal/ruleengine/condition"
	"telegram-message-forward/internal/ruleengine/processor"
)

type Service struct {
	repo      domainflow.Repository
	engine    *flowengine.Engine
	sources   domainsource.Repository
	sinks     domainsink.Repository
	templates domaintemplate.Repository
	filters   domainfilter.Repository
	messages  domainmessage.Repository
	tasks     domaindelivery.Repository
}

type ValidatorDeps struct {
	Messages  domainmessage.Repository
	Tasks     domaindelivery.Repository
	Sources   domainsource.Repository
	Sinks     domainsink.Repository
	Templates domaintemplate.Repository
	Filters   domainfilter.Repository
}

func NewService(repo domainflow.Repository, engine *flowengine.Engine, deps ...ValidatorDeps) *Service {
	s := &Service{repo: repo, engine: engine}
	if len(deps) > 0 {
		s.sources = deps[0].Sources
		s.sinks = deps[0].Sinks
		s.templates = deps[0].Templates
		s.filters = deps[0].Filters
		s.messages, s.tasks = deps[0].Messages, deps[0].Tasks
	}
	return s
}

type Input struct {
	Name        string
	Enabled     bool
	Priority    int
	StopOnMatch bool
	Nodes       []domainflow.Node
	Edges       []domainflow.Edge
}

type PreviewMessage struct {
	Links          []domainmessage.Link
	SentAt         *time.Time
	SourceID       int64
	MessageType    string
	SenderPeerType string
	SenderID       int64
	SenderName     string
	Text           string
	Media          []domainmessage.Media
	OriginalURL    string
	ReceivedAt     time.Time
}

type PreviewInput struct {
	FlowID    int64
	MessageID int64
	FullChain bool
	Edit      bool
	Flow      Input
	Message   PreviewMessage
}

type PreviewTarget struct {
	FlowID     int64                 `json:"flow_id"`
	NodeID     int64                 `json:"node_id"`
	Text       string                `json:"text"`
	Media      []domainmessage.Media `json:"media"`
	Status     string                `json:"status"`
	SinkID     int64
	TemplateID *int64
}

type PreviewResult struct {
	Evaluation    decision.Evaluation
	Matched       bool
	ProcessedText string
	Media         []domainmessage.Media
	Targets       []PreviewTarget
}

func (s *Service) List(ctx context.Context) ([]*domainflow.Flow, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*domainflow.Flow, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, in Input) (*domainflow.Flow, error) {
	f := toFlow(0, in)
	if err := s.validate(ctx, f); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, f); err != nil {
		return nil, err
	}
	s.engine.Invalidate(f.ID)
	return f, nil
}

func (s *Service) Update(ctx context.Context, id int64, in Input) (*domainflow.Flow, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	f := toFlow(existing.ID, in)
	f.CreatedAt = existing.CreatedAt
	if err := s.validate(ctx, f); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, f); err != nil {
		return nil, err
	}
	s.engine.Invalidate(f.ID)
	return f, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.engine.Invalidate(id)
	return nil
}

// Preview 用一条手工样例消息预演当前 Flow 草稿，不要求 Flow 先落库。
func (s *Service) Preview(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	if in.Edit && (in.MessageID <= 0 || s.tasks == nil) {
		return nil, fmt.Errorf("编辑模拟需要一条已采集的真实消息")
	}
	id := in.FlowID
	if id <= 0 {
		id = -1
	}
	f := toFlow(id, in.Flow)
	if !in.FullChain {
		f.Enabled = true
	}
	if err := s.validate(ctx, f); err != nil {
		return nil, err
	}
	msg := &domainmessage.NormalizedMessage{
		SourceID:       in.Message.SourceID,
		MessageType:    in.Message.MessageType,
		SenderPeerType: in.Message.SenderPeerType,
		SenderID:       in.Message.SenderID,
		SenderName:     in.Message.SenderName,
		Text:           in.Message.Text,
		Media:          in.Message.Media,
		OriginalURL:    in.Message.OriginalURL,
		ReceivedAt:     in.Message.ReceivedAt,
		Links:          in.Message.Links, SentAt: in.Message.SentAt,
	}
	if in.MessageID > 0 {
		if s.messages == nil {
			return nil, fmt.Errorf("真实消息查询不可用")
		}
		var err error
		msg, err = s.messages.GetByID(ctx, in.MessageID)
		if err != nil {
			return nil, err
		}
	}
	if msg.MessageType == "" {
		msg.MessageType = "text"
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now()
	}
	engine := flowengine.NewEngine(flowengine.WithFilterResolver(s.filters))
	flows := []*domainflow.Flow{f}
	if in.FullChain {
		loaded, err := s.repo.ListEnabledBySource(ctx, msg.SourceID)
		if err != nil {
			return nil, err
		}
		flows = []*domainflow.Flow{f}
		for _, other := range loaded {
			if in.FlowID <= 0 && other.ID >= f.ID {
				f.ID = other.ID + 1
			}
			if other.ID != in.FlowID {
				flows = append(flows, other)
			}
		}
	}
	matches, evaluation, err := engine.EvaluateDetailed(ctx, msg, flows)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return &PreviewResult{Evaluation: evaluation, Matched: false, ProcessedText: msg.Text, Media: msg.Media, Targets: []PreviewTarget{}}, nil
	}
	var routes []domaindelivery.FlowRoute
	if in.Edit {
		if in.MessageID <= 0 || s.tasks == nil {
			return nil, fmt.Errorf("编辑模拟需要一条已采集的真实消息")
		}
		routes, err = s.tasks.ListFlowRoutesByMessage(ctx, in.MessageID)
		if err != nil {
			return nil, err
		}
	}
	targets := make([]PreviewTarget, 0, len(matches))
	processedText := matches[0].Message.Text
	media := matches[0].Message.Media
	for _, match := range matches {
		for _, target := range match.Targets {
			status := "would_enqueue"
			if in.Edit {
				status = "skipped_no_historical_route"
				for _, route := range routes {
					if route.OriginID == match.OriginID && route.OriginNodeID == match.OriginNodeID && route.SinkID == target.SinkID {
						status = "would_enqueue"
						break
					}
				}
			}
			if s.sinks != nil {
				sink, err := s.routingSink(ctx, target.SinkID)
				if err != nil {
					return nil, err
				}
				if !sink.Enabled {
					status = "skipped_sink_disabled"
				}
			}
			targets = append(targets, PreviewTarget{FlowID: match.OriginID, NodeID: match.OriginNodeID, Text: match.Message.Text, Media: match.Message.Media, Status: status, SinkID: target.SinkID, TemplateID: target.TemplateID})
		}
	}
	return &PreviewResult{
		Evaluation:    evaluation,
		Matched:       true,
		ProcessedText: processedText,
		Media:         media,
		Targets:       targets,
	}, nil
}

func (s *Service) ConditionDescriptors() []condition.Descriptor {
	return condition.Descriptors()
}

func (s *Service) ProcessorDescriptors() []processor.Descriptor {
	return processor.Descriptors()
}

func toFlow(id int64, in Input) *domainflow.Flow {
	return &domainflow.Flow{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		Enabled:     in.Enabled,
		Priority:    in.Priority,
		StopOnMatch: in.StopOnMatch,
		Nodes:       in.Nodes,
		Edges:       in.Edges,
	}
}

func (s *Service) validate(ctx context.Context, f *domainflow.Flow) error {
	if f.Name == "" {
		return fmt.Errorf("flow 名称不能为空")
	}
	if utf8.RuneCountInString(f.Name) > 100 {
		return fmt.Errorf("flow 名称不能超过 100 个字符")
	}
	if s.engine == nil {
		s.engine = flowengine.NewEngine(flowengine.WithFilterResolver(s.filters))
	}
	if _, err := s.engine.Compile(f); err != nil {
		return err
	}
	for _, n := range f.Nodes {
		switch n.Type {
		case domainflow.NodeTypeSource:
			if s.sources != nil && n.RefID != nil {
				if _, err := s.sources.GetByID(ctx, *n.RefID); err != nil {
					return fmt.Errorf("来源节点(node=%d)引用的监听源不存在 id=%d: %w", n.ID, *n.RefID, err)
				}
			}
		case domainflow.NodeTypeFilter:
			if err := s.validateFilters(ctx, n.ID, n.Config.FilterIDs); err != nil {
				return err
			}
		case domainflow.NodeTypeTarget:
			if err := s.validateTarget(ctx, n); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) validateFilters(ctx context.Context, nodeID int64, ids []int64) error {
	if len(ids) == 0 || s.filters == nil {
		return nil
	}
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("过滤节点(node=%d)过滤器 id 无效: %d", nodeID, id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("过滤节点(node=%d)不能重复引用同一个过滤器 id=%d", nodeID, id)
		}
		seen[id] = struct{}{}
		if _, err := s.filters.GetByID(ctx, id); err != nil {
			return fmt.Errorf("过滤节点(node=%d)引用的过滤器不存在 id=%d: %w", nodeID, id, err)
		}
	}
	return nil
}

func (s *Service) validateTarget(ctx context.Context, n domainflow.Node) error {
	if s.sinks == nil || n.RefID == nil {
		return nil
	}
	sk, err := s.routingSink(ctx, *n.RefID)
	if err != nil {
		return fmt.Errorf("目标节点(node=%d)引用的渠道不存在 id=%d: %w", n.ID, *n.RefID, err)
	}
	format := domaintemplate.FormatText
	if n.TemplateID != nil {
		if s.templates == nil {
			return nil
		}
		tpl, err := s.templates.GetByID(ctx, *n.TemplateID)
		if err != nil {
			return fmt.Errorf("目标节点(node=%d)引用的模板不存在 id=%d: %w", n.ID, *n.TemplateID, err)
		}
		format = tpl.Format
	}
	if !sinkSupportsFormat(sk.Capabilities, format) {
		return fmt.Errorf("目标节点(node=%d)渠道 %s 不支持模板格式 %s", n.ID, sk.Type, format)
	}
	return nil
}

func (s *Service) routingSink(ctx context.Context, id int64) (*domainsink.Sink, error) {
	if reader, ok := s.sinks.(domainsink.RoutingReader); ok {
		return reader.GetRoutingInfo(ctx, id)
	}
	return s.sinks.GetByID(ctx, id)
}

func sinkSupportsFormat(c domainsink.Capabilities, f domaintemplate.Format) bool {
	switch f {
	case domaintemplate.FormatText:
		return c.SupportsText
	case domaintemplate.FormatMarkdown:
		return c.SupportsMarkdown
	case domaintemplate.FormatHTML:
		return c.SupportsHTML
	default:
		return false
	}
}
