package condition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"telegram-message-forward/internal/domain/decision"
	domainflow "telegram-message-forward/internal/domain/flow"
	domainmessage "telegram-message-forward/internal/domain/message"
)

const MaxConditions = 100
const MaxGroupDepth = 3

type Program struct {
	root *compiledCondition
	Hash string
}

type compiledCondition struct {
	cfg      domainflow.ConditionConfig
	impl     Condition
	regex    *regexp.Regexp
	children []*compiledCondition
}

var programCache = struct {
	sync.Mutex
	items map[string]*Program
}{items: map[string]*Program{}}

// Compile snapshots configs and caches only immutable validated programs.
func Compile(configs []domainflow.ConditionConfig) (*Program, error) {
	data, err := json.Marshal(configs)
	if err != nil || len(data) > 128*1024 {
		return nil, invalid("conditions", "config", "条件配置不能超过 128 KiB")
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	programCache.Lock()
	cached := programCache.items[hash]
	programCache.Unlock()
	if cached != nil {
		return cached, nil
	}
	var snapshot []domainflow.ConditionConfig
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	count := 0
	root, err := compileNode(domainflow.ConditionConfig{Type: "all", Children: snapshot}, 0, &count, "conditions")
	if err != nil {
		return nil, err
	}
	p := &Program{root: root, Hash: hash}
	programCache.Lock()
	if len(programCache.items) >= 256 {
		programCache.items = map[string]*Program{}
	}
	programCache.items[hash] = p
	programCache.Unlock()
	return p, nil
}

func compileNode(cfg domainflow.ConditionConfig, depth int, count *int, path string) (*compiledCondition, error) {
	*count++
	if *count > MaxConditions+1 || depth > MaxGroupDepth+1 {
		return nil, invalid(path, "children", "条件不能超过 100 个，分组不能超过 3 层")
	}
	node := &compiledCondition{cfg: cfg}
	if cfg.Type == "all" || cfg.Type == "any" {
		if depth > 0 && len(cfg.Children) == 0 {
			return nil, invalid(path, "children", "分组至少需要一个条件")
		}
		for i, child := range cfg.Children {
			compiled, err := compileNode(child, depth+1, count, fmt.Sprintf("%s.%d", path, i))
			if err != nil {
				return nil, err
			}
			node.children = append(node.children, compiled)
		}
		return node, nil
	}
	if len(cfg.Children) > 0 {
		return nil, invalid(path, "children", "只有 all/any 分组允许子条件")
	}
	if err := ValidateConfig(cfg.Type, cfg.Config); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := validateTextScope(cfg); err != nil {
		return nil, err
	}
	node.impl, _ = getRaw(cfg.Type)
	if cfg.Type == "regex" {
		node.regex, _ = regexp.Compile(configString(cfg.Config, "pattern"))
	}
	return node, nil
}

func validateTextScope(cfg domainflow.ConditionConfig) error {
	if fields, ok := cfg.Config["fields"]; ok {
		items, err := strictStrings(fields)
		if err != nil || len(items) == 0 {
			return invalid(cfg.Type, "config.fields", "匹配字段必须为非空字符串列表")
		}
		for _, field := range items {
			if !stringIn(field, []string{"text", "links", "media_names", "media_captions"}) {
				return invalid(cfg.Type, "config.fields", "不支持的匹配字段")
			}
		}
	}
	if mode := configString(cfg.Config, "normalize"); mode != "" && !stringIn(mode, []string{"none", "nfkc", "strip_invisible"}) {
		return invalid(cfg.Type, "config.normalize", "不支持的文本规范化方式")
	}
	if value, exists := cfg.Config["time_basis"]; exists && value != "sent" && value != "received" {
		return invalid(cfg.Type, "config.time_basis", "必须选择 sent 或 received")
	}
	return nil
}

func (p *Program) Evaluate(ctx context.Context, msg *domainmessage.NormalizedMessage) (decision.Condition, error) {
	if msg == nil {
		return decision.Condition{}, fmt.Errorf("过滤消息不能为空")
	}
	return p.root.evaluate(ctx, msg, "conditions", false)
}

func (n *compiledCondition) evaluate(ctx context.Context, msg *domainmessage.NormalizedMessage, path string, skipped bool) (decision.Condition, error) {
	r := decision.Condition{Path: path, Type: n.cfg.Type, Negate: n.cfg.Negate, Status: "skipped"}
	var err error
	passed := n.cfg.Type != "any"
	if len(n.children) > 0 || n.cfg.Type == "all" || n.cfg.Type == "any" {
		short := skipped
		for i, child := range n.children {
			cr, childErr := child.evaluate(ctx, msg, path+"."+strconv.Itoa(i), short)
			r.Children = append(r.Children, cr)
			if childErr != nil {
				err = childErr
				short = true
			}
			if short {
				continue
			}
			if n.cfg.Type == "all" && cr.Status != "passed" {
				passed, short = false, true
			} else if n.cfg.Type == "any" && cr.Status == "passed" {
				passed, short = true, true
			}
		}
	} else if !skipped {
		if err = ctx.Err(); err == nil {
			passed, r.Field, r.Evidence, err = n.evaluateLeaf(ctx, msg)
		}
	}
	if skipped {
		r.Reason = "short_circuit"
		return r, nil
	}
	if err != nil {
		r.Status, r.Reason = "error", "condition_execution_failed"
		return r, err
	}
	if n.cfg.Negate {
		passed = !passed
	}
	r.Status = "blocked"
	if passed {
		r.Status = "passed"
	}
	return r, nil
}

func (n *compiledCondition) evaluateLeaf(ctx context.Context, msg *domainmessage.NormalizedMessage) (bool, string, string, error) {
	if n.cfg.Type != "keyword_contains" && n.cfg.Type != "keyword_excludes" && n.cfg.Type != "regex" {
		passed, err := n.impl.Evaluate(ctx, msg, n.cfg.Config)
		return passed, "", "", err
	}
	fields := configStringSlice(n.cfg.Config, "fields")
	if len(fields) == 0 {
		fields = []string{"text"}
	}
	for _, field := range fields {
		for _, raw := range textValues(msg, field) {
			text := normalizedText(raw, n.cfg.Config)
			if n.regex != nil {
				if found := n.regex.FindStringIndex(text); found != nil {
					return true, field, safeEvidence(text[found[0]:found[1]], field), nil
				}
				continue
			}
			for _, keyword := range configStringSlice(n.cfg.Config, "keywords") {
				if strings.Contains(text, normalizedText(keyword, n.cfg.Config)) {
					return n.cfg.Type != "keyword_excludes", field, safeEvidence(keyword, field), nil
				}
			}
		}
	}
	return n.cfg.Type == "keyword_excludes", strings.Join(fields, ","), "", nil
}

func textValues(msg *domainmessage.NormalizedMessage, field string) []string {
	switch field {
	case "text":
		return []string{msg.Text}
	case "links":
		out := make([]string, 0, len(msg.Links))
		for _, link := range msg.Links {
			out = append(out, link.URL)
		}
		return out
	default:
		out := make([]string, 0, len(msg.Media))
		for _, media := range msg.Media {
			if field == "media_names" {
				out = append(out, media.FileName)
			} else {
				out = append(out, media.Caption)
			}
		}
		return out
	}
}

func normalizedText(text string, config map[string]any) string {
	switch configString(config, "normalize") {
	case "nfkc":
		text = norm.NFKC.String(text)
	case "strip_invisible":
		text = strings.Map(func(r rune) rune {
			if unicode.Is(unicode.Cf, r) {
				return -1
			}
			return r
		}, text)
	}
	if !configBool(config, "case_sensitive") && configString(config, "pattern") == "" {
		text = strings.ToLower(text)
	}
	return text
}

func safeEvidence(value, field string) string {
	if field == "links" {
		if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
			value = parsed.Hostname()
		} else if strings.ContainsAny(value, "?=&") {
			return "[URL match]"
		}
	}
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120]) + "..."
	}
	return value
}
