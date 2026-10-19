package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"telegram-message-forward/internal/domain/decision"
	"telegram-message-forward/internal/storage/model"
)

type DecisionRepository struct{ db *gorm.DB }

func NewDecisionRepository(db *gorm.DB) *DecisionRepository { return &DecisionRepository{db: db} }

func (r *DecisionRepository) Create(ctx context.Context, record *decision.Record) error {
	payload, err := marshalJSON(record.Evaluation)
	if err != nil {
		return err
	}
	if len(payload) > 1024*1024 {
		return fmt.Errorf("决策详情超过 1 MiB 上限")
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, flow := range record.Evaluation.Flows {
		for _, node := range flow.Nodes {
			for _, filter := range node.Filters {
				if !seen[filter.ID] {
					ids = append(ids, filter.ID)
					seen[filter.ID] = true
				}
			}
		}
	}
	filterIDs, err := marshalJSON(ids)
	if err != nil {
		return err
	}
	m := model.MessageDecision{MessageID: record.MessageID, SourceID: record.SourceID, MessageRevision: record.MessageRevision, Trigger: record.Trigger, Status: record.Evaluation.Status, FilterIDs: filterIDs, Evaluation: payload}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return err
	}
	record.ID, record.CreatedAt = m.ID, m.CreatedAt
	return nil
}

func (r *DecisionRepository) List(ctx context.Context, q decision.Query) ([]decision.Record, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.MessageDecision{})
	if q.MessageID > 0 {
		db = db.Where("message_id = ?", q.MessageID)
	}
	if q.SourceID > 0 {
		db = db.Where("source_id = ?", q.SourceID)
	}
	if q.FilterID > 0 {
		db = db.Where("filter_ids @> jsonb_build_array(?)", q.FilterID)
	}
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	var rows []model.MessageDecision
	if err := db.Order("id DESC").Limit(limit).Offset(q.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]decision.Record, 0, len(rows))
	for _, row := range rows {
		var evaluation decision.Evaluation
		if err := unmarshalJSON(row.Evaluation, &evaluation); err != nil {
			return nil, 0, err
		}
		out = append(out, decision.Record{ID: row.ID, MessageID: row.MessageID, SourceID: row.SourceID, MessageRevision: row.MessageRevision, Trigger: row.Trigger, Evaluation: evaluation, CreatedAt: row.CreatedAt})
	}
	return out, count, nil
}

func (r *DecisionRepository) DeleteBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	res := r.db.WithContext(ctx).Exec("DELETE FROM message_decisions WHERE id IN (SELECT id FROM message_decisions WHERE created_at < ? ORDER BY id LIMIT ?)", before, limit)
	return res.RowsAffected, res.Error
}
