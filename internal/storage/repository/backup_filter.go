package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"telegram-message-forward/internal/storage/model"
)

// Restoring configuration is a new local revision, not a rewind of audit history.
func restoreFilterRow(tx *gorm.DB, row *model.Filter, same bool) (int64, error) {
	backupID := row.ID
	row.Revision = 1
	if same && backupID > 0 {
		var current model.Filter
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, backupID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		if err == nil {
			row.Revision = current.Revision + 1
			row.CreatedAt = current.CreatedAt
		}
	}
	row.UpdatedAt = time.Now()
	id, err := saveConfigRow(tx, row, backupID, same)
	if err != nil {
		return 0, err
	}
	err = tx.Create(&model.FilterVersion{FilterID: id, Revision: row.Revision, Name: row.Name, Description: row.Description, Conditions: row.Conditions}).Error
	return id, err
}

// Restrictive references must never disappear into an unconditional configuration.
func remapRequiredJSONIDs(value datatypes.JSON, mapping map[int64]int64) (datatypes.JSON, error) {
	if len(value) == 0 {
		return value, nil
	}
	var ids []int64
	if err := json.Unmarshal(value, &ids); err != nil || strings.TrimSpace(string(value)) == "null" {
		return nil, errors.New("引用必须为 ID 数组")
	}
	for i, id := range ids {
		mapped, ok := mapping[id]
		if !ok || id <= 0 || mapped <= 0 {
			return nil, fmt.Errorf("引用 ID %d 不存在于备份中", id)
		}
		ids[i] = mapped
	}
	return json.Marshal(ids)
}

func remapFilterNodeConfig(value datatypes.JSON, sources, filters map[int64]int64) (datatypes.JSON, error) {
	if len(value) == 0 {
		return value, nil
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(value, &config); err != nil {
		return nil, err
	}
	if ids, ok := config["filter_ids"]; ok {
		remapped, err := remapRequiredJSONIDs(datatypes.JSON(ids), filters)
		if err != nil {
			return nil, err
		}
		config["filter_ids"] = json.RawMessage(remapped)
	}
	if conditions, ok := config["conditions"]; ok {
		remapped, err := remapFilterConditions(datatypes.JSON(conditions), sources)
		if err != nil {
			return nil, err
		}
		config["conditions"] = json.RawMessage(remapped)
	}
	return json.Marshal(config)
}

func remapFilterConditions(value datatypes.JSON, sources map[int64]int64) (datatypes.JSON, error) {
	if len(value) == 0 || strings.TrimSpace(string(value)) == "null" {
		return value, nil
	}
	var conditions []map[string]json.RawMessage
	if err := json.Unmarshal(value, &conditions); err != nil {
		return nil, err
	}
	for _, cond := range conditions {
		var kind string
		if err := json.Unmarshal(cond["type"], &kind); err != nil {
			return nil, err
		}
		if kind == "source" {
			var config map[string]json.RawMessage
			if err := json.Unmarshal(cond["config"], &config); err != nil {
				return nil, err
			}
			var ids []string
			if err := json.Unmarshal(config["source_ids"], &ids); err != nil {
				var single string
				if err := json.Unmarshal(config["source_ids"], &single); err != nil {
					return nil, errors.New("source_ids 必须为 ID 文本列表")
				}
				ids = []string{single}
			}
			if len(ids) == 0 {
				return nil, errors.New("source_ids 不能为空")
			}
			for i, text := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
				mapped, ok := sources[id]
				if err != nil || !ok || id <= 0 || mapped <= 0 {
					return nil, errors.New("source 条件引用不存在于备份中")
				}
				ids[i] = strconv.FormatInt(mapped, 10)
			}
			config["source_ids"], _ = json.Marshal(ids)
			cond["config"], _ = json.Marshal(config)
		}
		if children, ok := cond["children"]; ok {
			remapped, err := remapFilterConditions(datatypes.JSON(children), sources)
			if err != nil {
				return nil, err
			}
			cond["children"] = json.RawMessage(remapped)
		}
	}
	return json.Marshal(conditions)
}
