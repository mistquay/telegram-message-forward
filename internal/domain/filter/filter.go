// Package filter 定义可复用「过滤器」领域模型：一组命名的匹配条件，
// 供 Flow 与 AI 整理共同引用。过滤器只承载条件，不含处理器与目标渠道。
package filter

import (
	"context"
	"errors"
	"time"

	domainflow "telegram-message-forward/internal/domain/flow"
)

var ErrConflict = errors.New("过滤器版本冲突")
var ErrInUse = errors.New("过滤器仍被引用")

// Filter 是一组可复用的匹配条件。
type Filter struct {
	ID          int64
	Name        string
	Description string
	Conditions  []domainflow.ConditionConfig
	Revision    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Reference struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	NodeID  int64  `json:"node_id,omitempty"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type Version struct {
	Revision    int                          `json:"revision"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Conditions  []domainflow.ConditionConfig `json:"conditions"`
	CreatedAt   time.Time                    `json:"created_at"`
}

type Stats struct {
	Passed     int64     `json:"passed"`
	Blocked    int64     `json:"blocked"`
	Errors     int64     `json:"errors"`
	References int64     `json:"references"`
	Since      time.Time `json:"since"`
}

type DiagnosticsRepository interface {
	Version(context.Context, int64, int) (*Version, error)
	References(context.Context, int64) ([]Reference, error)
	Versions(context.Context, int64, int, int) ([]Version, int64, error)
	Stats(context.Context, time.Time) (map[int64]Stats, error)
}

// Repository 是过滤器仓储接口。
type Repository interface {
	List(ctx context.Context) ([]*Filter, error)
	GetByID(ctx context.Context, id int64) (*Filter, error)
	Create(ctx context.Context, f *Filter) error
	Update(ctx context.Context, f *Filter) error
	Delete(ctx context.Context, id int64) error
	// CountReferences 返回引用该过滤器的 Flow 节点与 AI Profile 总数。
	CountReferences(ctx context.Context, id int64) (int64, error)
}
