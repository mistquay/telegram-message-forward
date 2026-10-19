// Package decision defines bounded, versioned processing evidence without message bodies.
package decision

import (
	"context"
	"time"
)

type Condition struct {
	Path     string      `json:"path"`
	Type     string      `json:"type"`
	Status   string      `json:"status"` // passed | blocked | error | skipped
	Negate   bool        `json:"negate,omitempty"`
	Field    string      `json:"field,omitempty"`
	Evidence string      `json:"evidence,omitempty"`
	Reason   string      `json:"reason,omitempty"`
	Children []Condition `json:"children,omitempty"`
}

type Filter struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Revision int       `json:"revision"`
	Hash     string    `json:"hash"`
	Result   Condition `json:"result"`
}

type Node struct {
	NodeID     int64      `json:"node_id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	Filters    []Filter   `json:"filters,omitempty"`
	Conditions *Condition `json:"conditions,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	DurationUS int64      `json:"duration_us"`
}

type Flow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Nodes   []Node `json:"nodes"`
	Reason  string `json:"reason,omitempty"`
}

type Target struct {
	FlowID int64  `json:"flow_id"`
	NodeID int64  `json:"node_id"`
	SinkID int64  `json:"sink_id"`
	TaskID int64  `json:"task_id,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Evaluation struct {
	Version    int      `json:"version"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason,omitempty"`
	Flows      []Flow   `json:"flows"`
	Targets    []Target `json:"targets"`
	DurationUS int64    `json:"duration_us"`
}

type Record struct {
	ID              int64      `json:"id"`
	MessageID       int64      `json:"message_id"`
	SourceID        int64      `json:"source_id"`
	MessageRevision int        `json:"message_revision"`
	Trigger         string     `json:"trigger"`
	Evaluation      Evaluation `json:"evaluation"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Query struct {
	MessageID int64
	SourceID  int64
	FilterID  int64
	Status    string
	Limit     int
	Offset    int
}

type Repository interface {
	Create(context.Context, *Record) error
	List(context.Context, Query) ([]Record, int64, error)
	DeleteBefore(context.Context, time.Time, int) (int64, error)
}
