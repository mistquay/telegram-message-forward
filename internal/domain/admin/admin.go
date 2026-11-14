// Package admin 定义管理后台用户与浏览器会话。
package admin

import (
	"context"
	"errors"
	"time"
)

// ErrCredentialChanged 表示签发会话前用户名或密码已被更新，调用方必须重新登录。
var ErrCredentialChanged = errors.New("登录凭证已更新，请重新登录")

var ErrUsernameTaken = errors.New("该用户名已被使用，请换一个用户名")

// User 是管理后台用户。首版只做单管理员模型，但数据结构允许后续扩展。
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Active       bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session 是管理后台浏览器会话。Token 只在创建时明文返回，存储层只保存 hash。
type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Repository 是管理后台认证仓储接口。
type Repository interface {
	CountActiveUsers(ctx context.Context) (int64, error)
	CreateUserIfNoneActive(ctx context.Context, u *User) (bool, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	MarkUserLogin(ctx context.Context, userID int64, at time.Time) error

	// CreateSession 仅在用户名和密码哈希仍匹配时签发会话，与凭证修改操作互斥。
	CreateSession(ctx context.Context, s *Session, username, passwordHash string) error
	// ChangePassword 原子更新密码并吊销该用户全部会话；原凭证不匹配时返回 false。
	ChangePassword(ctx context.Context, userID int64, username, oldHash, newHash string) (bool, error)
	// ChangeUsername 原子更新用户名并吊销该用户全部会话；原凭证不匹配时返回 false。
	ChangeUsername(ctx context.Context, userID int64, oldUsername, passwordHash, newUsername string) (bool, error)
	GetActiveSessionByHash(ctx context.Context, hash string, now time.Time) (*Session, *User, error)
	TouchSession(ctx context.Context, id int64, at time.Time) error
	RevokeSessionByHash(ctx context.Context, hash string) error
}
