package repository

import (
	"context"
	"time"

	domainfilter "telegram-message-forward/internal/domain/filter"
	"telegram-message-forward/internal/storage/model"
)

func (r *FilterRepository) References(ctx context.Context, id int64) ([]domainfilter.Reference, error) {
	out := []domainfilter.Reference{}
	err := r.db.WithContext(ctx).Raw(`
SELECT 'flow' AS kind,f.id,n.id AS node_id,f.name,f.enabled
FROM flows f JOIN flow_nodes n ON n.flow_id=f.id
WHERE n.type='filter' AND COALESCE(n.config->'filter_ids','[]'::jsonb) @> jsonb_build_array(?::bigint)
UNION ALL
SELECT 'ai_digest',p.id,0,p.name,p.enabled FROM ai_digest_profiles p
WHERE p.filter_id=? OR COALESCE(p.filter_ids,'[]'::jsonb) @> jsonb_build_array(?::bigint)
ORDER BY kind,id,node_id`, id, id, id).Scan(&out).Error
	return out, err
}

func (r *FilterRepository) Versions(ctx context.Context, id int64, limit, offset int) ([]domainfilter.Version, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.FilterVersion{}).Where("filter_id = ?", id)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.FilterVersion
	if err := db.Order("revision DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domainfilter.Version, 0, len(rows))
	for _, row := range rows {
		v := domainfilter.Version{Revision: row.Revision, Name: row.Name, Description: row.Description, CreatedAt: row.CreatedAt}
		if err := unmarshalJSON(row.Conditions, &v.Conditions); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, count, nil
}

func (r *FilterRepository) Version(ctx context.Context, id int64, revision int) (*domainfilter.Version, error) {
	var row model.FilterVersion
	if err := r.db.WithContext(ctx).Where("filter_id = ? AND revision = ?", id, revision).First(&row).Error; err != nil {
		return nil, err
	}
	version := &domainfilter.Version{Revision: row.Revision, Name: row.Name, Description: row.Description, CreatedAt: row.CreatedAt}
	if err := unmarshalJSON(row.Conditions, &version.Conditions); err != nil {
		return nil, err
	}
	return version, nil
}

func (r *FilterRepository) Stats(ctx context.Context, since time.Time) (map[int64]domainfilter.Stats, error) {
	type stat struct {
		ID      int64
		Passed  int64
		Blocked int64
		Errors  int64
	}
	var rows []stat
	err := r.db.WithContext(ctx).Raw(`
WITH executions AS (
SELECT (f->>'id')::bigint AS id,(f->>'passed')::bigint AS passed,(f->>'blocked')::bigint AS blocked,(f->>'errors')::bigint AS errors
FROM message_decisions d CROSS JOIN LATERAL jsonb_array_elements(COALESCE(d.evaluation->'filter_totals','[]'::jsonb)) f WHERE d.created_at>=?
UNION ALL
SELECT (f->>'id')::bigint AS id,
CASE WHEN f->'result'->>'status'='passed' THEN 1 ELSE 0 END AS passed,
CASE WHEN f->'result'->>'status'='blocked' THEN 1 ELSE 0 END AS blocked,
CASE WHEN f->'result'->>'status'='error' THEN 1 ELSE 0 END AS errors
FROM message_decisions d
CROSS JOIN LATERAL jsonb_array_elements(d.evaluation->'flows') flow
CROSS JOIN LATERAL jsonb_array_elements(flow->'nodes') node
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(node->'filters','[]'::jsonb)) f
WHERE d.created_at>=? AND NOT jsonb_exists(d.evaluation,'filter_totals')
) SELECT id,SUM(passed) AS passed,SUM(blocked) AS blocked,SUM(errors) AS errors FROM executions GROUP BY id`, since, since).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[int64]domainfilter.Stats{}
	for _, row := range rows {
		out[row.ID] = domainfilter.Stats{Passed: row.Passed, Blocked: row.Blocked, Errors: row.Errors, Since: since}
	}
	type referenceCount struct {
		ID    int64
		Count int64
	}
	var refs []referenceCount
	err = r.db.WithContext(ctx).Raw(`
SELECT id,COUNT(*) AS count FROM (
SELECT jsonb_array_elements_text(COALESCE(config->'filter_ids','[]'::jsonb))::bigint AS id FROM flow_nodes WHERE type='filter'
UNION ALL
SELECT jsonb_array_elements_text(CASE WHEN filter_ids <> '[]'::jsonb THEN filter_ids WHEN filter_id IS NOT NULL THEN jsonb_build_array(filter_id) ELSE '[]'::jsonb END)::bigint AS id FROM ai_digest_profiles
) r GROUP BY id`).Scan(&refs).Error
	for _, row := range refs {
		s := out[row.ID]
		s.References = row.Count
		s.Since = since
		out[row.ID] = s
	}
	return out, err
}
