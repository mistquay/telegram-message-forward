package condition

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"telegram-message-forward/internal/domain/formschema"
)

// ValidationError exposes a field path without echoing user-provided values.
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Path + ": " + e.Message }

func invalid(name, field, message string) error {
	return &ValidationError{Path: name + "." + field, Message: message}
}

func validateFields(name string, fields []formschema.FieldSpec, config map[string]any) error {
	encoded, err := json.Marshal(config)
	if err != nil || len(encoded) > 65536 {
		return invalid(name, "config", "配置必须为有效 JSON，且不超过 64 KiB")
	}
	for _, field := range fields {
		value, present := config[field.Key]
		if !present || value == nil {
			continue
		}
		bad := func(message string) error { return invalid(name, "config."+field.Key, message) }
		switch field.Type {
		case formschema.FieldText, formschema.FieldTextarea:
			text, ok := value.(string)
			if !ok || (field.Required && strings.TrimSpace(text) == "") {
				return bad("必须为非空文本")
			}
		case formschema.FieldBoolean:
			if _, ok := value.(bool); !ok {
				return bad("必须为布尔值")
			}
		case formschema.FieldStringList, formschema.FieldMultiSelect:
			items, err := strictStrings(value)
			if err != nil {
				return bad(err.Error())
			}
			if field.Required && len(items) == 0 {
				return bad("至少选择一项")
			}
			for _, item := range items {
				if strings.TrimSpace(item) == "" {
					return bad("不能包含空项")
				}
				if len(field.Options) > 0 {
					valid := false
					for _, option := range field.Options {
						valid = valid || item == option.Value
					}
					if !valid {
						return bad("包含不支持的选项")
					}
				}
				if field.Key == "source_ids" || field.Key == "sender_ids" {
					id, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
					if err != nil || id <= 0 {
						return bad("ID 必须为正整数文本，Telegram ID 使用内部原始 ID")
					}
				}
			}
		case formschema.FieldNumber:
			n, ok := nonnegativeInteger(value)
			if !ok || n > 10000000 {
				return bad("必须为 0 至 10000000 的整数")
			}
		}
	}
	if name == "sender" && len(configStringSlice(config, "sender_ids"))+len(configStringSlice(config, "peer_types"))+len(configStringSlice(config, "names")) == 0 {
		return invalid(name, "config", "发送者类型、ID、名称至少配置一项")
	}
	if name == "message_length" {
		_, minSet := config["min"]
		_, maxSet := config["max"]
		if !minSet && !maxSet {
			return invalid(name, "config", "至少配置最小或最大长度")
		}
		if max := configInt(config, "max"); max > 0 && configInt(config, "min") > max {
			return invalid(name, "config.max", "最大长度不能小于最小长度")
		}
	}
	return nil
}

func strictStrings(value any) ([]string, error) {
	var out []string
	switch v := value.(type) {
	case string:
		out = []string{v}
	case []string:
		out = v
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("必须为字符串列表")
			}
			out = append(out, s)
		}
	default:
		return nil, fmt.Errorf("必须为字符串列表")
	}
	if len(out) > 2000 {
		return nil, fmt.Errorf("列表不能超过 2000 项")
	}
	return out, nil
}

func nonnegativeInteger(value any) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
		if err != nil {
			return 0, false
		}
		n = float64(parsed)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n == math.Trunc(n)
}
