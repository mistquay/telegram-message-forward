package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainfilter "telegram-message-forward/internal/domain/filter"
	domainflow "telegram-message-forward/internal/domain/flow"
	"telegram-message-forward/internal/storage/model"
)

// FilterRepository 是 filter.Repository 的 PostgreSQL 实现。
type FilterRepository struct {
	db *gorm.DB
}

func NewFilterRepository(db *gorm.DB) *FilterRepository {
	return &FilterRepository{db: db}
}

var _ domainfilter.Repository = (*FilterRepository)(nil)

func (r *FilterRepository) List(ctx context.Context) ([]*domainfilter.Filter, error) {
	var ms []model.Filter
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*domainfilter.Filter, 0, len(ms))
	for i := range ms {
		f, err := toFilterDomain(&ms[i])
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *FilterRepository) GetByID(ctx context.Context, id int64) (*domainfilter.Filter, error) {
	var m model.Filter
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return toFilterDomain(&m)
}

func (r *FilterRepository) Create(ctx context.Context, f *domainfilter.Filter) error {
	m, err := toFilterModel(f)
	if err != nil {
		return err
	}
	m.Revision = 1
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Create(&model.FilterVersion{FilterID: m.ID, Revision: 1, Name: m.Name, Description: m.Description, Conditions: m.Conditions}).Error
	}); err != nil {
		return err
	}
	f.ID = m.ID
	f.Revision = m.Revision
	f.CreatedAt = m.CreatedAt
	f.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *FilterRepository) Update(ctx context.Context, f *domainfilter.Filter) error {
	conds, err := marshalJSON(f.Conditions)
	if err != nil {
		return err
	}
	now := time.Now()
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Filter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, f.ID).Error; err != nil {
			return err
		}
		if f.Revision > 0 && f.Revision != existing.Revision {
			return fmt.Errorf("%w，请刷新后重试", domainfilter.ErrConflict)
		}
		f.Revision = existing.Revision + 1
		if err := tx.Model(&model.Filter{}).Where("id = ?", f.ID).Updates(map[string]any{
			"name":        f.Name,
			"description": f.Description,
			"conditions":  conds,
			"updated_at":  now,
			"revision":    f.Revision,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.FilterVersion{FilterID: f.ID, Revision: f.Revision, Name: f.Name, Description: f.Description, Conditions: conds}).Error
	}); err != nil {
		return err
	}
	f.UpdatedAt = now
	return nil
}

func (r *FilterRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Filter{}, id).Error
}

func (r *FilterRepository) CountReferences(ctx context.Context, id int64) (int64, error) {
	var flowCount int64
	if err := r.db.WithContext(ctx).
		Model(&model.FlowNode{}).
		Where("type = ?", string(domainflow.NodeTypeFilter)).
		Where("COALESCE(config->'filter_ids', '[]'::jsonb) @> jsonb_build_array(?::bigint)", id).
		Count(&flowCount).Error; err != nil {
		return 0, err
	}
	var profileCount int64
	// 兼容旧 filter_id 与新 filter_ids jsonb 数组。
	if err := r.db.WithContext(ctx).Model(&model.AIDigestProfile{}).
		Where("filter_id = ? OR COALESCE(filter_ids, '[]'::jsonb) @> jsonb_build_array(?::bigint)", id, id).
		Count(&profileCount).Error; err != nil {
		return 0, err
	}
	return flowCount + profileCount, nil
}

func toFilterModel(f *domainfilter.Filter) (*model.Filter, error) {
	conds, err := marshalJSON(f.Conditions)
	if err != nil {
		return nil, err
	}
	return &model.Filter{
		ID:          f.ID,
		Name:        f.Name,
		Description: f.Description,
		Conditions:  conds,
		Revision:    f.Revision,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
	}, nil
}

func toFilterDomain(m *model.Filter) (*domainfilter.Filter, error) {
	var conds []domainflow.ConditionConfig
	if err := unmarshalJSON(m.Conditions, &conds); err != nil {
		return nil, err
	}
	return &domainfilter.Filter{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Conditions:  conds,
		Revision:    m.Revision,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}, nil
}

func (r *FilterRepository) GetByIDs(ctx context.Context, ids []int64) ([]*domainfilter.Filter, error) {
	var rows []model.Filter
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domainfilter.Filter, 0, len(rows))
	for i := range rows {
		f, err := toFilterDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// loadFilterConditions 一次查询取回多个过滤器的条件。
func loadFilterConditions(ctx context.Context, db *gorm.DB, ids []int64) (map[int64][]domainflow.ConditionConfig, error) {
	out := map[int64][]domainflow.ConditionConfig{}
	if len(ids) == 0 {
		return out, nil
	}
	var ms []model.Filter
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&ms).Error; err != nil {
		return nil, err
	}
	for i := range ms {
		var conds []domainflow.ConditionConfig
		if err := unmarshalJSON(ms[i].Conditions, &conds); err != nil {
			return nil, err
		}
		out[ms[i].ID] = conds
	}
	return out, nil
}
