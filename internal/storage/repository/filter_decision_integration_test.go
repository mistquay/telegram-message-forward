package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	appfilter "telegram-message-forward/internal/app/filter"
	appflow "telegram-message-forward/internal/app/flow"
	appingest "telegram-message-forward/internal/app/ingest"
	"telegram-message-forward/internal/dispatch"
	domainaidigest "telegram-message-forward/internal/domain/aidigest"
	"telegram-message-forward/internal/domain/decision"
	domaindelivery "telegram-message-forward/internal/domain/delivery"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	domainsink "telegram-message-forward/internal/domain/sink"
	domainsource "telegram-message-forward/internal/domain/source"
	"telegram-message-forward/internal/flowengine"
	"telegram-message-forward/internal/infra/clock"
	"telegram-message-forward/internal/infra/crypto"
	"telegram-message-forward/internal/storage/migrate"
	"telegram-message-forward/internal/storage/model"
	"telegram-message-forward/internal/storage/repository"
)

func TestFilterBackupDatabaseRoundTrip(t *testing.T) {
	db := filterTestDatabase(t)
	ctx := context.Background()
	cipher, err := crypto.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	backups := repository.NewBackupRepository(db, cipher)
	source := &domainsource.Source{Type: "webhook", PeerType: "webhook", PeerID: 77, Name: "backup source", Enabled: true}
	if err := repository.NewSourceRepository(db).Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	f := &domainfilter.Filter{Name: "source restriction", Conditions: []domainflow.ConditionConfig{{Type: "source", Config: map[string]any{"source_ids": []string{fmt.Sprint(source.ID)}}}}}
	filters := repository.NewFilterRepository(db)
	if err := filters.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	flow := &domainflow.Flow{Name: "backup flow", Enabled: true, Nodes: []domainflow.Node{
		{ID: -1, Type: domainflow.NodeTypeSource, RefID: &source.ID},
		{ID: -2, Type: domainflow.NodeTypeFilter, Config: domainflow.NodeConfig{FilterIDs: []int64{f.ID}}},
		{ID: -3, Type: domainflow.NodeTypeFilter, Config: domainflow.NodeConfig{Conditions: f.Conditions}},
	}, Edges: []domainflow.Edge{{FromNodeID: -1, ToNodeID: -2}, {FromNodeID: -2, ToNodeID: -3}}}
	flows := repository.NewFlowRepository(db)
	if err := flows.Create(ctx, flow); err != nil {
		t.Fatal(err)
	}
	profile := &domainaidigest.Profile{Name: "backup profile", SourceIDs: []int64{source.ID}, FilterIDs: []int64{f.ID}, Conditions: f.Conditions, PromptTemplate: "{{messages}}", OutputFormat: "text"}
	if err := repository.NewAIDigestRepository(db).CreateProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	payload, _, err := backups.ExportPayload(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	f.Name = "newer local name"
	if err := filters.Update(ctx, f); err != nil {
		t.Fatal(err)
	}
	if _, err := backups.RestorePayload(ctx, payload); err != nil {
		t.Fatal(err)
	}
	restored, err := filters.GetByID(ctx, f.ID)
	if err != nil || restored.Revision != 3 || restored.Name != "source restriction" {
		t.Fatalf("restore must append local revision: %+v %v", restored, err)
	}
	v2, err := filters.Version(ctx, f.ID, 2)
	if err != nil || v2.Name != "newer local name" {
		t.Fatalf("restore must preserve existing history: %+v %v", v2, err)
	}
	loadedFlow, err := flows.GetByID(ctx, flow.ID)
	if err != nil || loadedFlow.Nodes[1].ID != flow.Nodes[1].ID {
		t.Fatalf("same-installation restore must preserve route identity: %+v %v", loadedFlow, err)
	}
	// An old backup has no revision metadata. Cross-installation IDs must be remapped.
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	var badProfiles []model.AIDigestProfile
	if err := json.Unmarshal(decoded["ai_profiles"], &badProfiles); err != nil {
		t.Fatal(err)
	}
	originalProfiles := decoded["ai_profiles"]
	badProfiles[0].FilterIDs = datatypes.JSON(`[999999]`)
	decoded["ai_profiles"], _ = json.Marshal(badProfiles)
	badPayload, _ := json.Marshal(decoded)
	if _, err := backups.RestorePayload(ctx, badPayload); err == nil {
		t.Fatal("restore must not silently drop a missing restriction")
	}
	after, err := filters.GetByID(ctx, f.ID)
	if err != nil || after.Revision != 3 {
		t.Fatalf("failed restore must roll back filter revisions: %+v %v", after, err)
	}
	decoded["ai_profiles"] = originalProfiles
	var rows []model.Filter
	if err := json.Unmarshal(decoded["filters"], &rows); err != nil {
		t.Fatal(err)
	}
	rows[0].Revision = 0
	decoded["filters"], _ = json.Marshal(rows)
	legacyPayload, _ := json.Marshal(decoded)
	targetDB := filterTestDatabase(t)
	for _, table := range []string{"sources", "filters"} {
		if err := targetDB.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), 100, true)", table).Error; err != nil {
			t.Fatal(err)
		}
	}
	targetBackups := repository.NewBackupRepository(targetDB, cipher)
	if _, err := targetBackups.RestorePayload(ctx, legacyPayload); err != nil {
		t.Fatal(err)
	}
	targetFilters := repository.NewFilterRepository(targetDB)
	newFilters, err := targetFilters.List(ctx)
	if err != nil || len(newFilters) != 1 || newFilters[0].ID <= 100 || newFilters[0].Revision != 1 {
		t.Fatalf("new installation needs v1 baseline: %+v %v", newFilters, err)
	}
	refs, err := targetFilters.References(ctx, newFilters[0].ID)
	if err != nil || len(refs) != 2 {
		t.Fatalf("Flow and AI references must be remapped: %+v %v", refs, err)
	}
	if newFilters[0].Conditions[0].Config["source_ids"].([]any)[0] != "101" {
		t.Fatal("shared source restriction was not remapped")
	}
	newFlows, err := repository.NewFlowRepository(targetDB).List(ctx)
	if err != nil || newFlows[0].Nodes[2].Config.Conditions[0].Config["source_ids"].([]any)[0] != "101" {
		t.Fatalf("inline source restriction was not remapped: %v", err)
	}
}

// This test never reads production config. Each run owns a fresh isolated schema.
func filterTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("TMF_FILTER_TEST_DSN")
	if raw == "" {
		t.Skip("设置 TMF_FILTER_TEST_DSN 为独立本机 PostgreSQL URL 运行过滤集成测试")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("过滤集成测试只允许显式指定本机 postgres:// URL")
	}
	admin, err := gorm.Open(postgres.New(postgres.Config{DSN: raw, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("连接隔离测试数据库失败: %v", err)
	}
	sqlAdmin, _ := admin.DB()
	sqlAdmin.SetMaxOpenConns(1)
	// Extensions are database-wide; keep their operators visible to concurrent test schemas.
	if err := admin.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public").Error; err != nil {
		_ = sqlAdmin.Close()
		t.Fatal(err)
	}
	schema := fmt.Sprintf("tmf_filter_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	_ = sqlAdmin.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: raw, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开测试 schema 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		_ = sqlDB.Close()
	})
	if err := db.Exec("SET search_path TO " + schema + ", public").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(context.Background(), sqlDB, "up"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFilterDecisionDatabaseRoundTrip(t *testing.T) {
	db := filterTestDatabase(t)
	ctx := context.Background()
	filters := repository.NewFilterRepository(db)
	f := &domainfilter.Filter{Name: "exclude spam", Conditions: []domainflow.ConditionConfig{{Type: "keyword_excludes", Config: map[string]any{"keywords": []string{"spam"}}}}}
	if err := filters.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	old := *f
	f.Description = "version two"
	if err := filters.Update(ctx, f); err != nil {
		t.Fatal(err)
	}
	if f.Revision != 2 {
		t.Fatalf("revision=%d", f.Revision)
	}
	if err := filters.Update(ctx, &old); err == nil {
		t.Fatal("应拒绝旧版本覆盖")
	}
	versions, total, err := filters.Versions(ctx, f.ID, 30, 0)
	if err != nil || total != 2 || len(versions) != 2 {
		t.Fatalf("versions=%+v total=%d err=%v", versions, total, err)
	}
	firstVersion, err := filters.Version(ctx, f.ID, 1)
	if err != nil || firstVersion.Revision != 1 || firstVersion.Description != "" {
		t.Fatalf("历史版本必须保持原内容: %+v %v", firstVersion, err)
	}
	sources := repository.NewSourceRepository(db)
	source := &domainsource.Source{Type: "webhook", PeerType: "webhook", PeerID: 1, Name: "isolated source", Enabled: true}
	if err := sources.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	sink := model.Sink{Type: "webhook", Name: "not sent", Enabled: true, Capabilities: datatypes.JSON(`{"supports_text":true}`)}
	if err := db.Omit("config").Create(&sink).Error; err != nil {
		t.Fatal(err)
	}
	sourceID, sinkID := source.ID, sink.ID
	flow := &domainflow.Flow{Name: "shared", Enabled: true, Nodes: []domainflow.Node{
		{ID: -1, Type: domainflow.NodeTypeSource, RefID: &sourceID},
		{ID: -2, Type: domainflow.NodeTypeFilter, Config: domainflow.NodeConfig{FilterIDs: []int64{f.ID}}},
		{ID: -3, Type: domainflow.NodeTypeTarget, RefID: &sinkID},
	}, Edges: []domainflow.Edge{{FromNodeID: -1, ToNodeID: -2}, {FromNodeID: -2, ToNodeID: -3}}}
	flows := repository.NewFlowRepository(db)
	if err := flows.Create(ctx, flow); err != nil {
		t.Fatal(err)
	}
	targetID := flow.Nodes[2].ID
	flow.Name = "shared updated"
	if err := flows.Update(ctx, flow); err != nil {
		t.Fatal(err)
	}
	if flow.Nodes[2].ID != targetID {
		t.Fatal("保存 Flow 不能改变保留节点身份")
	}
	aiRepo := repository.NewAIDigestRepository(db)
	profile := &domainaidigest.Profile{Name: "shared AI reference", SourceIDs: []int64{source.ID}, FilterIDs: []int64{f.ID}, PromptTemplate: "{{messages}}", OutputFormat: "text"}
	if err := aiRepo.CreateProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	refs, err := filters.References(ctx, f.ID)
	if err != nil || len(refs) != 2 {
		t.Fatalf("refs=%+v err=%v", refs, err)
	}
	count, err := filters.CountReferences(ctx, f.ID)
	if err != nil || count != 2 {
		t.Fatalf("refs=%d err=%v", count, err)
	}
	if err := appfilter.NewService(filters).Delete(ctx, f.ID); err == nil {
		t.Fatal("被引用过滤器不能删除")
	}
	messages := repository.NewMessageRepository(db)
	tasks := repository.NewDeliveryRepository(db)
	decisions := repository.NewDecisionRepository(db)
	engine := flowengine.NewEngine(flowengine.WithFilterResolver(filters))
	notifier := dispatch.NewNotifier()
	service := appingest.NewService(messages, flows, engine, dispatch.NewQueue(tasks, 3, notifier), clock.System{}, slog.New(slog.NewTextHandler(io.Discard, nil))).UseDecisions(decisions).UseProcessingTransaction(repository.NewProcessingTransaction(db))
	m := &domainmessage.NormalizedMessage{SourceID: source.ID, ExternalMessageID: 1, Text: "safe", MessageType: "text"}
	if err := service.Ingest(ctx, m); err != nil {
		t.Fatal(err)
	}
	edited := &domainmessage.NormalizedMessage{SourceID: source.ID, ExternalMessageID: 1, Text: "spam", MessageType: "text", EventKind: domainmessage.EventEdit}
	if err := service.Ingest(ctx, edited); err != nil {
		t.Fatal(err)
	}
	var taskCount int64
	db.Model(&model.DeliveryTask{}).Count(&taskCount)
	if taskCount != 1 {
		t.Fatalf("编辑过滤应阻止补发，tasks=%d", taskCount)
	}
	existingTasks, err := tasks.List(ctx, "", 10, 0)
	if err != nil || len(existingTasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", existingTasks, err)
	}
	duplicate := *existingTasks[0]
	duplicate.ID = 0
	duplicate.CreationDisposition = ""
	if err := tasks.Create(ctx, &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.CreationDisposition != "duplicate" || duplicate.ID != existingTasks[0].ID {
		t.Fatal("幂等冲突必须显式返回重复任务状态")
	}
	records, n, err := decisions.List(ctx, decision.Query{MessageID: m.ID, FilterID: f.ID, Limit: 30})
	if err != nil || n != 2 {
		t.Fatalf("decisions=%+v n=%d err=%v", records, n, err)
	}
	if records[0].MessageRevision != 2 || records[1].Evaluation.Status != "enqueued" {
		t.Fatalf("决策快照错误: %+v", records)
	}
	run := &domainaidigest.Run{ProfileID: profile.ID, Status: domainaidigest.RunSuccess, TriggerType: domainaidigest.TriggerManual, WindowStart: time.Now().Add(-time.Hour), WindowEnd: time.Now()}
	if err := aiRepo.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	trace := decision.Node{Type: "filter", Status: "blocked"}
	for _, step := range records[0].Evaluation.Flows[0].Nodes {
		if step.Type == "filter" {
			trace = step
			break
		}
	}
	if err := aiRepo.AddRunItems(ctx, []*domainaidigest.RunItem{{RunID: run.ID, MessageID: m.ID, SourceID: source.ID, Message: m, FilterTrace: trace, Reason: "condition_not_match"}}); err != nil {
		t.Fatal(err)
	}
	items, err := aiRepo.ListRunItems(ctx, run.ID)
	if err != nil || len(items) != 1 || len(items[0].FilterTrace.Filters) != 1 {
		t.Fatalf("AI 判断记录必须完整往返: %+v %v", items, err)
	}
	flowSvc := appflow.NewService(flows, engine, appflow.ValidatorDeps{Sources: sources, Sinks: repository.NewSinkRepository(db, nil), Filters: filters, Messages: messages, Tasks: tasks})
	preview, err := flowSvc.Preview(ctx, appflow.PreviewInput{FlowID: flow.ID, MessageID: m.ID, FullChain: true, Flow: appflow.Input{Name: flow.Name, Enabled: true, Nodes: flow.Nodes, Edges: flow.Edges}})
	if err != nil || len(preview.Targets) != 0 {
		t.Fatalf("按当前编辑后的 spam 内容预演应阻断 %+v %v", preview, err)
	}
	var untouched model.Sink
	if err := db.First(&untouched, sink.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(untouched.ConfigEncrypted) > 0 {
		t.Fatal("只读预演不能触发渠道凭证懒迁移")
	}
	var afterPreviewTasks int64
	db.Model(&model.DeliveryTask{}).Count(&afterPreviewTasks)
	if afterPreviewTasks != taskCount {
		t.Fatal("预演不能创建任务")
	}
	stats, err := filters.Stats(ctx, time.Now().Add(-time.Hour))
	if err != nil || stats[f.ID].Passed != 1 || stats[f.ID].Blocked != 1 || stats[f.ID].References != 2 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	select {
	case <-notifier.C():
	default:
		t.Fatal("成功提交新任务后应唤醒 worker")
	}
	service.UseProcessingTransaction(failingAuditTransaction{inner: repository.NewProcessingTransaction(db)})
	failed := &domainmessage.NormalizedMessage{SourceID: source.ID, ExternalMessageID: 2, Text: "safe but audit fails", MessageType: "text"}
	if err := service.Ingest(ctx, failed); err == nil {
		t.Fatal("审计写入失败应返回错误并回滚任务")
	}
	var afterRollback int64
	db.Model(&model.DeliveryTask{}).Count(&afterRollback)
	if afterRollback != taskCount {
		t.Fatal("决策写入失败不能留下可投递的新任务")
	}
	failures, _, err := decisions.List(ctx, decision.Query{MessageID: failed.ID})
	if err != nil || len(failures) != 1 || failures[0].Evaluation.Reason != "dispatch_rolled_back" {
		t.Fatalf("应保留回滚失败决策 %+v %v", failures, err)
	}
	select {
	case <-notifier.C():
		t.Fatal("事务未提交时不能唤醒 worker")
	default:
	}
	if err := db.Exec("UPDATE message_decisions SET created_at = ?", time.Now().Add(-40*24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := decisions.DeleteBefore(ctx, time.Now().Add(-30*24*time.Hour), 1)
	if err != nil || deleted != 1 {
		t.Fatalf("bounded cleanup n=%d err=%v", deleted, err)
	}
	sqlDB, _ := db.DB()
	if err := migrate.Run(ctx, sqlDB, "down-to", "33"); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(ctx, sqlDB, "up"); err != nil {
		t.Fatal(err)
	}
}

type failingAuditTransaction struct {
	inner appingest.ProcessingTransaction
}

func (t failingAuditTransaction) Within(ctx context.Context, fn func(domaindelivery.Repository, decision.Repository, domainsink.RoutingReader) error) error {
	return t.inner.Within(ctx, func(tasks domaindelivery.Repository, records decision.Repository, reader domainsink.RoutingReader) error {
		return fn(tasks, failingAuditRepository{Repository: records}, reader)
	})
}

type failingAuditRepository struct{ decision.Repository }

func (failingAuditRepository) Create(context.Context, *decision.Record) error {
	return errors.New("simulated audit failure")
}
