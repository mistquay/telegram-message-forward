package dispatch

import (
	"context"
	"testing"
	"time"

	domaindelivery "telegram-message-forward/internal/domain/delivery"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	domainsink "telegram-message-forward/internal/domain/sink"
)

type notifyTaskRepo struct {
	created int
	tasks   []*domaindelivery.Task
	routes  []domaindelivery.FlowRoute
}

func (r *notifyTaskRepo) Create(_ context.Context, t *domaindelivery.Task) error {
	r.created++
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *notifyTaskRepo) Claim(context.Context, string, int) ([]*domaindelivery.Task, error) {
	return nil, nil
}
func (r *notifyTaskRepo) UpdateStatus(context.Context, *domaindelivery.Task) error { return nil }
func (r *notifyTaskRepo) RecoverStale(context.Context, time.Time) (int64, error)   { return 0, nil }
func (r *notifyTaskRepo) Requeue(context.Context, int64) error                     { return nil }
func (r *notifyTaskRepo) AddAttempt(context.Context, *domaindelivery.Attempt) error {
	return nil
}
func (r *notifyTaskRepo) GetByID(context.Context, int64) (*domaindelivery.Task, error) {
	return nil, nil
}
func (r *notifyTaskRepo) List(context.Context, domaindelivery.Status, int, int) ([]*domaindelivery.Task, error) {
	return nil, nil
}
func (r *notifyTaskRepo) Count(context.Context, domaindelivery.Status) (int64, error) {
	return int64(r.created), nil
}
func (r *notifyTaskRepo) ListFlowRoutesByMessage(context.Context, int64) ([]domaindelivery.FlowRoute, error) {
	return r.routes, nil
}

func TestQueueNotifiesAfterEnqueue(t *testing.T) {
	notifier := NewNotifier()
	repo := &notifyTaskRepo{}
	queue := NewQueue(repo, 3, notifier)

	msg := &domainmessage.NormalizedMessage{ID: 10}
	matches := []domainflow.Match{{
		FlowID:       20,
		Targets:      []domainflow.Target{{SinkID: 30}},
		Message:      msg,
		OriginID:     20,
		OriginNodeID: 40,
	}}
	if err := queue.Enqueue(context.Background(), msg, matches); err != nil {
		t.Fatal(err)
	}
	if repo.created != 1 {
		t.Fatalf("created = %d, want 1", repo.created)
	}

	select {
	case <-notifier.C():
	case <-time.After(time.Second):
		t.Fatal("enqueue 后应唤醒 worker")
	}
}

func TestQueueFlowMatchLeavesRuleIDEmpty(t *testing.T) {
	repo := &notifyTaskRepo{}
	queue := NewQueue(repo, 3)

	msg := &domainmessage.NormalizedMessage{ID: 10}
	matches := []domainflow.Match{{
		FlowID:       7,
		Targets:      []domainflow.Target{{SinkID: 30}},
		Message:      msg,
		OriginType:   "flow",
		OriginID:     7,
		OriginNodeID: 42,
	}}
	if err := queue.Enqueue(context.Background(), msg, matches); err != nil {
		t.Fatal(err)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("created = %d, want 1", len(repo.tasks))
	}
	task := repo.tasks[0]
	if task.RuleID != 0 {
		t.Fatalf("flow 任务不应写 rule_id，got %d", task.RuleID)
	}
	if task.OriginType != "flow" || task.OriginID != 7 || task.OriginNodeID != 42 {
		t.Fatalf("origin 字段错误: %+v", task)
	}
}

func TestQueueSkipsDisabledSink(t *testing.T) {
	notifier := NewNotifier()
	repo := &notifyTaskRepo{}
	queue := NewQueue(repo, 3, notifier).UseSinks(fakeSinkRepo{sink: &domainsink.Sink{ID: 30, Enabled: false}})

	msg := &domainmessage.NormalizedMessage{ID: 10}
	matches := []domainflow.Match{{
		FlowID:       20,
		Targets:      []domainflow.Target{{SinkID: 30}},
		Message:      msg,
		OriginID:     20,
		OriginNodeID: 40,
	}}
	if err := queue.Enqueue(context.Background(), msg, matches); err != nil {
		t.Fatal(err)
	}
	if repo.created != 0 {
		t.Fatalf("禁用渠道不应创建投递任务，created = %d", repo.created)
	}

	select {
	case <-notifier.C():
		t.Fatal("没有创建任务时不应唤醒 worker")
	default:
	}
}

func TestQueueEnqueueEditReusesOriginalRoutes(t *testing.T) {
	templateID := int64(9)
	repo := &notifyTaskRepo{routes: []domaindelivery.FlowRoute{{
		SinkID: 30, TemplateID: &templateID, OriginID: 7, OriginNodeID: 42,
	}}}
	queue := NewQueue(repo, 3)
	msg := &domainmessage.NormalizedMessage{ID: 10, Text: "edited", ContentRevision: 2}

	matches := []domainflow.Match{{OriginID: 7, OriginNodeID: 42, Message: msg, Targets: []domainflow.Target{{SinkID: 30}}}}
	if err := queue.EnqueueEdit(context.Background(), msg, matches, "--edited"); err != nil {
		t.Fatal(err)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("created = %d, want 1", len(repo.tasks))
	}
	task := repo.tasks[0]
	if task.SinkID != 30 || task.OriginID != 7 || task.OriginNodeID != 42 {
		t.Fatalf("未复用原 Flow 路由: %+v", task)
	}
	if task.MessageRevision != 2 || task.TextSuffix != "--edited" {
		t.Fatalf("编辑 revision/后缀错误: %+v", task)
	}
	if task.MessageSnapshot == nil || task.MessageSnapshot.Text != "edited" {
		t.Fatalf("编辑消息快照错误: %+v", task.MessageSnapshot)
	}
}

func TestQueueEditDoesNotBypassRejectedFilter(t *testing.T) {
	repo := &notifyTaskRepo{routes: []domaindelivery.FlowRoute{{SinkID: 30, OriginID: 7, OriginNodeID: 42}}}
	msg := &domainmessage.NormalizedMessage{ID: 10, Text: "spam", ContentRevision: 2}
	if err := NewQueue(repo, 3).EnqueueEdit(context.Background(), msg, nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(repo.tasks) != 0 {
		t.Fatal("历史路由不能绕过当前过滤结果")
	}
}

func TestQueueEditNeverAddsNewRoutesAndUsesProcessedSnapshot(t *testing.T) {
	newTemplate := int64(99)
	repo := &notifyTaskRepo{routes: []domaindelivery.FlowRoute{{SinkID: 30, OriginID: 7, OriginNodeID: 42}}}
	msg := &domainmessage.NormalizedMessage{ID: 10, Text: "raw", ContentRevision: 2}
	processed := *msg
	processed.Text = "processed"
	matches := []domainflow.Match{
		{OriginID: 7, OriginNodeID: 42, Message: &processed, Targets: []domainflow.Target{{SinkID: 30, TemplateID: &newTemplate}}},
		{OriginID: 8, OriginNodeID: 43, Message: msg, Targets: []domainflow.Target{{SinkID: 31}}},
	}
	if err := NewQueue(repo, 3).EnqueueEdit(context.Background(), msg, matches, ""); err != nil {
		t.Fatal(err)
	}
	if len(repo.tasks) != 1 || repo.tasks[0].MessageSnapshot.Text != "processed" {
		t.Fatalf("应仅向原路由投递当前处理快照: %+v", repo.tasks)
	}
	if repo.tasks[0].TemplateID == nil || *repo.tasks[0].TemplateID != newTemplate {
		t.Fatal("编辑补发应使用当前目标模板，而非旧任务模板")
	}
}

func TestQueueEditDeduplicatesHistoricalTemplatesForSameRoute(t *testing.T) {
	one, two := int64(1), int64(2)
	repo := &notifyTaskRepo{routes: []domaindelivery.FlowRoute{{SinkID: 30, OriginID: 7, OriginNodeID: 42, TemplateID: &one}, {SinkID: 30, OriginID: 7, OriginNodeID: 42, TemplateID: &two}}}
	msg := &domainmessage.NormalizedMessage{ID: 10, ContentRevision: 3}
	matches := []domainflow.Match{{OriginID: 7, OriginNodeID: 42, Targets: []domainflow.Target{{SinkID: 30}}, Message: msg}}
	if err := NewQueue(repo, 3).EnqueueEdit(context.Background(), msg, matches, ""); err != nil {
		t.Fatal(err)
	}
	if len(repo.tasks) != 1 {
		t.Fatal("同一路由历史模板变化不能重复补发")
	}
}

type duplicateTaskRepo struct{ notifyTaskRepo }

func (r *duplicateTaskRepo) Create(_ context.Context, task *domaindelivery.Task) error {
	task.ID = 9
	task.CreationDisposition = "duplicate"
	return nil
}

func TestQueueReportsDuplicateWithoutClaimingNewEnqueue(t *testing.T) {
	notifier := NewNotifier()
	repo := &duplicateTaskRepo{}
	msg := &domainmessage.NormalizedMessage{ID: 1}
	results, err := NewQueue(repo, 3, notifier).EnqueueDetailed(context.Background(), msg, []domainflow.Match{{OriginID: 2, OriginNodeID: 3, Message: msg, Targets: []domainflow.Target{{SinkID: 4}}}})
	if err != nil || len(results) != 1 || results[0].Status != "deduplicated" {
		t.Fatalf("%+v %v", results, err)
	}
	select {
	case <-notifier.C():
		t.Fatal("未产生新任务时不应声称入队并唤醒")
	default:
	}
}
