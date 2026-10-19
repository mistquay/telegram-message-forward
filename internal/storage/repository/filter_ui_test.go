package repository_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"telegram-message-forward/internal/api/dto"
	"telegram-message-forward/internal/api/handler"
	appfilter "telegram-message-forward/internal/app/filter"
	appflow "telegram-message-forward/internal/app/flow"
	appingest "telegram-message-forward/internal/app/ingest"
	appmessage "telegram-message-forward/internal/app/message"
	"telegram-message-forward/internal/dispatch"
	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
	domainsource "telegram-message-forward/internal/domain/source"
	"telegram-message-forward/internal/flowengine"
	"telegram-message-forward/internal/infra/clock"
	"telegram-message-forward/internal/storage/model"
	"telegram-message-forward/internal/storage/repository"
)

// Opt-in local browser fixture: real services and SQL, no workers or external providers.
func TestFilterUIHarness(t *testing.T) {
	addr := os.Getenv("TMF_FILTER_UI_ADDR")
	if addr == "" {
		t.Skip("仅在显式启用本机浏览器测试时启动")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("浏览器测试只允许绑定 127.0.0.1")
	}
	webDir := os.Getenv("TMF_FILTER_UI_WEB_DIR")
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
		t.Fatal("先构建 web 并设置 TMF_FILTER_UI_WEB_DIR")
	}
	db := filterTestDatabase(t)
	ctx := context.Background()
	filters := repository.NewFilterRepository(db)
	sources := repository.NewSourceRepository(db)
	flows := repository.NewFlowRepository(db)
	messages := repository.NewMessageRepository(db)
	decisions := repository.NewDecisionRepository(db)
	tasks := repository.NewDeliveryRepository(db)
	f := &domainfilter.Filter{Name: "广告排除", Description: "正文排除广告与 spam，仅作用于引用分支", Conditions: []domainflow.ConditionConfig{{Type: "keyword_excludes", Config: map[string]any{"keywords": []string{"广告", "spam"}}}}}
	if err := filters.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	other := &domainfilter.Filter{Name: "重要通知", Description: "按正文包含 urgent 或重要通知", Conditions: []domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"urgent", "重要通知"}}}}}
	if err := filters.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	source := &domainsource.Source{Type: "webhook", PeerType: "webhook", PeerID: 7, Name: "隔离样例来源", Enabled: true}
	if err := sources.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	sink := model.Sink{Type: "webhook", Name: "隔离目标（未启动发送）", Enabled: true, Config: datatypes.JSON(`{}`), Capabilities: datatypes.JSON(`{"supports_text":true}`)}
	if err := db.Create(&sink).Error; err != nil {
		t.Fatal(err)
	}
	sourceID, sinkID := source.ID, sink.ID
	fw := &domainflow.Flow{Name: "并联路径示例", Enabled: true, Priority: 10, StopOnMatch: true, Nodes: []domainflow.Node{
		{ID: -1, Type: domainflow.NodeTypeSource, RefID: &sourceID, PosX: 0, PosY: 80},
		{ID: -2, Type: domainflow.NodeTypeFilter, Config: domainflow.NodeConfig{FilterIDs: []int64{f.ID}}, PosX: 280, PosY: 0},
		{ID: -3, Type: domainflow.NodeTypeFilter, Config: domainflow.NodeConfig{Conditions: other.Conditions}, PosX: 280, PosY: 180},
		{ID: -4, Type: domainflow.NodeTypeTarget, RefID: &sinkID, PosX: 560, PosY: 80},
	}, Edges: []domainflow.Edge{{FromNodeID: -1, ToNodeID: -2}, {FromNodeID: -1, ToNodeID: -3}, {FromNodeID: -2, ToNodeID: -4}, {FromNodeID: -3, ToNodeID: -4}}}
	if err := flows.Create(ctx, fw); err != nil {
		t.Fatal(err)
	}
	engine := flowengine.NewEngine(flowengine.WithFilterResolver(filters))
	ingest := appingest.NewService(messages, flows, engine, dispatch.NewQueue(tasks, 3), clock.System{}, slog.New(slog.NewTextHandler(io.Discard, nil))).UseDecisions(decisions).UseProcessingTransaction(repository.NewProcessingTransaction(db))
	for i, text := range []string{"正常消息", "spam 广告", "urgent spam"} {
		if err := ingest.Ingest(ctx, &domainmessage.NormalizedMessage{SourceID: source.ID, ExternalMessageID: int64(i + 1), Text: text, MessageType: "text", SenderID: 77, SenderName: "测试发送者", SenderPeerType: "user"}); err != nil {
			t.Fatal(err)
		}
	}
	// Keep v1 execution evidence while exposing a different current version in the UI.
	f.Description = "v2 增加推广拦截；历史执行仍保留 v1"
	f.Conditions = []domainflow.ConditionConfig{{Type: "keyword_excludes", Config: map[string]any{"keywords": []string{"广告", "spam", "推广"}}}}
	if err := filters.Update(ctx, f); err != nil {
		t.Fatal(err)
	}
	legacy := &domainfilter.Filter{Name: "媒体默认条件（旧格式）", Description: "未显式 config，默认要求有附件", Conditions: []domainflow.ConditionConfig{{Type: "has_media"}}}
	if err := filters.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	group := &domainfilter.Filter{Name: "条件分组示例", Description: "任一满足：包含 go 或无附件", Conditions: []domainflow.ConditionConfig{{Type: "any", Children: []domainflow.ConditionConfig{{Type: "keyword_contains", Config: map[string]any{"keywords": []string{"go"}}}, {Type: "has_media", Negate: true}}}}}
	if err := filters.Create(ctx, group); err != nil {
		t.Fatal(err)
	}
	fsvc := appfilter.NewService(filters).UseDiagnostics(appfilter.DiagnosticsDeps{Messages: messages, Queries: messages, Decisions: decisions})
	flowSvc := appflow.NewService(flows, engine, appflow.ValidatorDeps{Sources: sources, Sinks: repository.NewSinkRepository(db, nil), Filters: filters, Messages: messages, Tasks: tasks})
	fh := handler.NewFilterHandler(fsvc)
	flh := handler.NewFlowHandler(flowSvc)
	mh := handler.NewMessageHandler(appmessage.NewService(messages).UseDecisions(decisions))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/v1/auth/me", func(c *gin.Context) { c.JSON(200, gin.H{"data": gin.H{"auth_enabled": false, "authenticated": false}}) })
	r.GET("/api/v1/filters", fh.List)
	r.POST("/api/v1/filters", fh.Create)
	r.POST("/api/v1/filters/preview", fh.Preview)
	r.POST("/api/v1/filters/validate", fh.Validate)
	r.GET("/api/v1/filters/:id", fh.Get)
	r.PUT("/api/v1/filters/:id", fh.Update)
	r.DELETE("/api/v1/filters/:id", fh.Delete)
	r.GET("/api/v1/filters/:id/references", fh.References)
	r.GET("/api/v1/filters/:id/versions", fh.Versions)
	r.GET("/api/v1/filters/:id/versions/:revision", fh.Version)
	r.GET("/api/v1/filters/:id/decisions", fh.Decisions)
	r.GET("/api/v1/flows", flh.List)
	r.GET("/api/v1/flows/meta", flh.Meta)
	r.POST("/api/v1/flows/preview", flh.Preview)
	r.GET("/api/v1/flows/:id", flh.Get)
	r.POST("/api/v1/flows", flh.Create)
	r.PUT("/api/v1/flows/:id", flh.Update)
	r.GET("/api/v1/messages", mh.List)
	r.GET("/api/v1/messages/:id", mh.Get)
	r.GET("/api/v1/sources", func(c *gin.Context) { c.JSON(200, gin.H{"data": []dto.SourceDTO{dto.NewSourceDTO(source)}}) })
	r.GET("/api/v1/sinks", func(c *gin.Context) {
		c.JSON(200, gin.H{"data": []gin.H{{"id": sink.ID, "name": sink.Name, "type": "webhook", "enabled": true, "capabilities": gin.H{"supports_text": true}, "config": gin.H{}}}})
	})
	r.GET("/api/v1/sinks/meta", func(c *gin.Context) {
		c.JSON(200, gin.H{"data": []gin.H{{"type": "webhook", "label": "Webhook", "fields": []any{}}}})
	})
	for _, path := range []string{"/api/v1/accounts", "/api/v1/templates"} {
		r.GET(path, func(c *gin.Context) { c.JSON(200, gin.H{"data": []any{}}) })
	}
	r.Static("/assets", filepath.Join(webDir, "assets"))
	r.NoRoute(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			c.JSON(404, gin.H{"error": "not provided by isolated fixture"})
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: r, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	errors := make(chan error, 1)
	go func() { errors <- server.Serve(listener) }()
	t.Logf("ISOLATED_UI http://%s/filters (real services, memory PostgreSQL, no outbound delivery)", addr)
	stopFile := os.Getenv("TMF_FILTER_UI_STOP_FILE")
	timer := time.NewTimer(15 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-errors:
			if err != http.ErrServerClosed {
				t.Fatal(err)
			}
			return
		case <-timer.C:
			return
		case <-ticker.C:
			if stopFile != "" {
				if _, err := os.Stat(stopFile); err == nil {
					return
				}
			}
		}
	}
}
