<script setup lang="ts">
import type { ConditionConfig, RuleItemDescriptor } from '@/types'
import { conditionSummary } from '@/utils/filterConditions'
defineOptions({ name: 'ConditionDetails' })
const props = withDefaults(defineProps<{ conditions: ConditionConfig[]; descriptors: RuleItemDescriptor[]; depth?: number }>(), { depth: 0 })
const labels: Record<string, string> = { keywords: '关键词', pattern: 'Go 正则', case_sensitive: '区分大小写', fields: '检查范围', normalize: '文本规范化', time_basis: '判断时间', start: '开始时间', end: '结束时间', timezone: '时区', min: '最小长度', max: '最大长度', value: '包含附件', peer_types: '发送者类型', names: '名称包含', types: '类型', source_ids: '来源 ID', sender_ids: '发送者原始 ID' }
const values: Record<string, string> = { text: '正文', links: '链接地址', media_names: '附件名称', media_captions: '附件说明', none: '保持原文', nfkc: '全半角兼容 NFKC', strip_invisible: '移除不可见格式字符', sent: '发送时间（缺失时回退接收时间）', received: '接收时间' }
function entries(c: ConditionConfig): [string, unknown][] {
  const cfg = { ...(c.config ?? {}) }
  if (c.type.startsWith('keyword_') || c.type === 'regex') { cfg.fields ??= ['text']; cfg.normalize ??= 'none'; if (c.type !== 'regex') cfg.case_sensitive ??= false }
  if (c.type === 'time_window') { cfg.time_basis ??= 'sent'; cfg.timezone ||= '服务器时区（未指定）' }
  if (c.type === 'has_media') cfg.value ??= true
  return Object.entries(cfg)
}
function label(c: ConditionConfig, key: string) { return labels[key] ?? props.descriptors.find((d) => d.type === c.type)?.fields.find((f) => f.key === key)?.label ?? key }
function format(c: ConditionConfig, key: string, value: unknown): string {
  if (Array.isArray(value)) return value.map((v) => format(c, key, v)).join('、') || '未限制'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (key === 'max' && value === 0) return '0（不设上限）'
  if (typeof value !== 'string') return JSON.stringify(value)
  if (['fields','normalize','time_basis'].includes(key)) return values[value] ?? value
  const option = props.descriptors.find((d) => d.type === c.type)?.fields.find((f) => f.key === key)?.options?.find((o) => o.value === value)
  return option ? `${option.label} [${value}]` : value
}
</script>

<template>
  <div class="condition-details">
    <details v-for="(c, i) in conditions" :key="i" :open="depth === 0">
      <summary>{{ i + 1 }}. {{ conditionSummary(c) }}</summary>
      <ConditionDetails v-if="c.children" :conditions="c.children" :descriptors="descriptors" :depth="depth + 1" />
      <dl v-else><div v-for="[key, value] in entries(c)" :key="key"><dt>{{ label(c, key) }}</dt><dd>{{ format(c, key, value) }}</dd></div></dl>
    </details>
  </div>
</template>

<style scoped>
.condition-details { display: grid; gap: 10px; margin: 14px 0; min-width: 0; }
details { border: 1px solid var(--clay-border); padding: 12px; border-radius: 8px; min-width: 0; }
summary { font-weight: 600; cursor: pointer; overflow-wrap: anywhere; }
dl { margin: 10px 0 0; display: grid; gap: 8px; } dl > div { display: grid; grid-template-columns: 125px minmax(0, 1fr); gap: 12px; }
dt { color: var(--clay-text-3); } dd { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
@media(max-width: 560px) { dl > div { grid-template-columns: 1fr; gap: 2px; } }
</style>
