import type { ConditionConfig, RuleItemDescriptor } from '@/types'

export const conditionNames: Record<string, string> = {
  all: '全部满足 (AND)', any: '任一满足 (OR)', keyword_contains: '包含任一关键词',
  keyword_excludes: '不包含任何排除词', regex: '正则匹配', source: '来源属于', sender: '发送者',
  message_type: '消息类型', media_type: '附件类型', has_media: '是否包含附件', message_length: '正文长度', time_window: '每日时间窗口',
}

export function conditionSummary(c: ConditionConfig): string {
  const prefix = c.negate ? '取反：' : ''
  if (c.type === 'all' || c.type === 'any') return prefix + `(${(c.children ?? []).map(conditionSummary).join(c.type === 'all' ? ' 且 ' : ' 或 ')})`
  const cfg = c.config ?? {}
  const values = cfg.keywords ?? cfg.types ?? cfg.source_ids ?? cfg.sender_ids ?? cfg.names ?? cfg.pattern
  const value = Array.isArray(values) ? values.join('、') : values === undefined ? '' : String(values)
  const detail = c.type === 'message_length' ? `${cfg.min ?? 0}–${Number(cfg.max) > 0 ? cfg.max : '不限'}`
    : c.type === 'time_window' ? `${cfg.start ?? '?'}–${cfg.end ?? '?'} ${cfg.timezone || '服务器时区'} (${cfg.time_basis === 'received' ? '接收' : '发送'}时间)`
      : c.type === 'has_media' ? (cfg.value === false ? '无附件' : '有附件')
        : c.type === 'sender' ? ['peer_types','sender_ids','names'].filter((k) => Array.isArray(cfg[k]) && (cfg[k] as unknown[]).length).map((k) => `${({ peer_types: '类型', sender_ids: 'ID', names: '名称包含' } as Record<string, string>)[k]}=${(cfg[k] as unknown[]).join('、')}`).join('；') : value
  const fields = Array.isArray(cfg.fields) ? ` [${cfg.fields.join('+')}]` : ''
  const options = `${cfg.case_sensitive === true ? ' · 区分大小写' : ''}${cfg.normalize && cfg.normalize !== 'none' ? ` · ${cfg.normalize}` : ''}`
  return prefix + (conditionNames[c.type] ?? c.type) + (detail ? `：${detail}` : '') + fields + options
}

export function cloneConditions(conditions: ConditionConfig[]): ConditionConfig[] {
  const copy = JSON.parse(JSON.stringify(conditions)) as ConditionConfig[]
  const visit = (items: ConditionConfig[]) => items.forEach((c) => { c.config ??= {}; if (c.children) visit(c.children) })
  visit(copy)
  return copy
}

export function newCondition(type: string, descriptors: RuleItemDescriptor[]): ConditionConfig {
  return { type, config: Object.fromEntries((descriptors.find((d) => d.type === type)?.fields ?? []).filter((f) => f.default !== undefined).map((f) => [f.key, JSON.parse(JSON.stringify(f.default))])), ...(type === 'all' || type === 'any' ? { children: [] } : {}) }
}

export function validateCondition(c: ConditionConfig): string {
  const cfg = c.config ?? {}
  if (c.type === 'all' || c.type === 'any') return c.children?.length ? '' : '分组至少需要一个子条件'
  if (['keyword_contains', 'keyword_excludes', 'source', 'message_type', 'media_type'].includes(c.type)) {
    const key = c.type.startsWith('keyword_') ? 'keywords' : c.type === 'source' ? 'source_ids' : 'types'
    if (!Array.isArray(cfg[key]) || !(cfg[key] as unknown[]).length) return '至少填写或选择一项'
  }
  if (c.type === 'sender' && !['sender_ids', 'peer_types', 'names'].some((k) => Array.isArray(cfg[k]) && (cfg[k] as unknown[]).length)) return '发送者类型、ID、名称至少配置一项'
  for (const key of ['sender_ids', 'source_ids']) {
    if (Array.isArray(cfg[key]) && (cfg[key] as unknown[]).some((id) => !/^[1-9]\d*$/.test(String(id).trim()))) return 'ID 必须为正整数；使用内部原始 ID，不是 @用户名或 -100 前缀'
  }
  if (c.type === 'message_length' && Number(cfg.max) > 0 && Number(cfg.min) > Number(cfg.max)) return '最大长度不能小于最小长度'
  if (c.type === 'regex' && !String(cfg.pattern ?? '').trim()) return '请填写 Go 正则表达式'
  return ''
}
