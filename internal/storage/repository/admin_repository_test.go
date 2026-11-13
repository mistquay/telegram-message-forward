package repository_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domainadmin "telegram-message-forward/internal/domain/admin"
	"telegram-message-forward/internal/storage/model"
	"telegram-message-forward/internal/storage/repository"
)

// 只连接显式指定的本机测试数据库，并为每个测试创建独立 schema；不读取项目真实配置。
func adminTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("TMF_AUTH_TEST_DSN")
	if raw == "" {
		t.Skip("设置 TMF_AUTH_TEST_DSN 为本机 PostgreSQL URL 运行认证仓储集成测试")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("认证集成测试只允许显式指定本机 postgres:// URL")
	}
	open := func(dsn string) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal("无法连接本机认证测试数据库")
		}
		return db
	}
	admin := open(raw)
	sqlAdmin, _ := admin.DB()
	t.Cleanup(func() { _ = sqlAdmin.Close() })
	schema := fmt.Sprintf("tmf_auth_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db := open(u.String())
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	// 使用正式迁移中管理账号和会话的 DDL，无需创建 Telegram 等无关表。
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "00004_admin_and_shared_telegram_config.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(strings.SplitN(string(sql), "CREATE TABLE telegram_apps", 2)[0]).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func seedAdminSession(t *testing.T, db *gorm.DB, name string) (*model.AdminUser, *domainadmin.Session) {
	t.Helper()
	user := &model.AdminUser{Username: name, PasswordHash: "old-test-hash", Active: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	sess := &domainadmin.Session{UserID: user.ID, TokenHash: "test-session-" + name, ExpiresAt: time.Now().Add(time.Hour)}
	if err := repository.NewAdminRepository(db).CreateSession(context.Background(), sess, user.PasswordHash); err != nil {
		t.Fatal(err)
	}
	return user, sess
}

func TestAdminPasswordChangeRevokesOnlyTargetSessions(t *testing.T) {
	db := adminTestDatabase(t)
	repo := repository.NewAdminRepository(db)
	user, sess := seedAdminSession(t, db, "target")
	_, otherSess := seedAdminSession(t, db, "other")
	ctx := context.Background()
	if changed, err := repo.ChangePassword(ctx, user.ID, "old-test-hash", "new-test-hash"); err != nil || !changed {
		t.Fatalf("password change failed: changed=%t err=%v", changed, err)
	}
	if current, _, err := repo.GetActiveSessionByHash(ctx, sess.TokenHash, time.Now()); err != nil || current != nil {
		t.Fatal("target session must be revoked")
	}
	if current, _, err := repo.GetActiveSessionByHash(ctx, otherSess.TokenHash, time.Now()); err != nil || current == nil {
		t.Fatal("another user's session must remain active")
	}
	stale := &domainadmin.Session{UserID: user.ID, TokenHash: "stale-session", ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.CreateSession(ctx, stale, "old-test-hash"); !errors.Is(err, domainadmin.ErrCredentialChanged) {
		t.Fatalf("old password version must not issue a session: %v", err)
	}
	if changed, err := repo.ChangePassword(ctx, user.ID, "old-test-hash", "unexpected-hash"); err != nil || changed {
		t.Fatal("a stale concurrent password update must be rejected")
	}
	if err := repo.CreateSession(ctx, stale, "new-test-hash"); err != nil {
		t.Fatal("new password version must issue a session")
	}
}

func TestAdminPasswordChangeRollsBackWhenSessionRevocationFails(t *testing.T) {
	db := adminTestDatabase(t)
	user, sess := seedAdminSession(t, db, "rollback")
	// 故意阻止会话吊销，验证密码更新也必须回滚，避免只更新了一半。
	if err := db.Exec("ALTER TABLE admin_sessions ADD CONSTRAINT test_block_revocation CHECK (revoked_at IS NULL)").Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewAdminRepository(db)
	if changed, err := repo.ChangePassword(context.Background(), user.ID, "old-test-hash", "new-test-hash"); err == nil || changed {
		t.Fatal("session revocation failure must fail the password change")
	}
	var stored model.AdminUser
	if err := db.First(&stored, user.ID).Error; err != nil || stored.PasswordHash != "old-test-hash" {
		t.Fatal("failed session revocation must roll back the password hash")
	}
	if current, _, err := repo.GetActiveSessionByHash(context.Background(), sess.TokenHash, time.Now()); err != nil || current == nil {
		t.Fatal("rolled-back change must preserve the original session")
	}
}
