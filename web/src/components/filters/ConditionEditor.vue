<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import ConfigFormRenderer from '@/components/ConfigFormRenderer.vue'
import { filtersApi, sourcesApi, messagesApi } from '@/api/client'
import { errText } from '@/utils/error'
import type { ConditionConfig, RuleItemDescriptor } from '@/types'
import { conditionNames, conditionSummary, newCondition, validateCondition } from '@/utils/filterConditions'

defineOptions({ name: 'ConditionEditor' })
const model = defineModel<ConditionConfig[]>({ required: true })
const props = withDefaults(defineProps<{
  descriptors: RuleItemDescriptor[]
  depth?: number
  sourceOptions?: { label: string; value: string }[]
  senderOptions?: { label: string; value: string }[]
}>(), { depth: 0 })

const localSources = ref<{ label: string; value: string }[]>([])
const localSenders = ref<{ label: string; value: string }[]>([])
const editedKeywords = new WeakSet<ConditionConfig>()
const serverError = ref('')
let validationTimer: ReturnType<typeof setTimeout> | undefined
let validationGeneration = 0
watch(model, () => {
  if (props.depth !== 0) return
  const generation = ++validationGeneration
  clearTimeout(validationTimer)
  serverError.value = ''
  if (!model.value.length) return
  validationTimer = setTimeout(async () => {
    try { await filtersApi.validate(model.value) }
    catch (e) { if (generation === validationGeneration) serverError.value = errText(e) }
  }, 500)
}, { deep: true, immediate: true })
onBeforeUnmount(() => { clearTimeout(validationTimer); validationGeneration++ })
const sources = computed(() => props.sourceOptions ?? localSources.value)
const senders = computed(() => props.senderOptions ?? localSenders.value)
const options = computed(() => [
  ...props.descriptors.map((d) => ({ label: conditionNames[d.type] ?? d.label, value: d.type })),
  ...(props.depth < 3 ? [{ label: conditionNames.all!, value: 'all' }, { label: conditionNames.any!, value: 'any' }] : []),
])
const scopes = [{ label: '正文', value: 'text' }, { label: '链接地址', value: 'links' }, { label: '附件名称', value: 'media_names' }, { label: '附件说明', value: 'media_captions' }]
const normalizations = [{ label: '保持原文（兼容旧配置）', value: 'none' }, { label: '全半角兼容 NFKC', value: 'nfkc' }, { label: '移除不可见格式字符', value: 'strip_invisible' }]

onMounted(async () => {
  if (props.depth !== 0) return
  const [ss, ms] = await Promise.allSettled([sourcesApi.list(), messagesApi.page({ limit: 100 })])
  if (ss.status === 'fulfilled') localSources.value = ss.value.map((s) => ({ label: `${s.name} · #${s.id}`, value: String(s.id) }))
  if (ms.status === 'fulfilled') localSenders.value = [...new Map(ms.value.data.filter((m) => m.sender_id).map((m) => [String(m.sender_id), { label: `${m.sender_name || '未记录名称'} · ${m.sender_peer_type} #${m.sender_id}`, value: String(m.sender_id) }])).values()]
})

function changeType(index: number, type: string) { model.value[index] = newCondition(type, props.descriptors) }
function add() { model.value = [...model.value, newCondition('keyword_excludes', props.descriptors)] }
function fields(c: ConditionConfig) {
  return (props.descriptors.find((d) => d.type === c.type)?.fields ?? []).filter((f) => !['keywords','source_ids','sender_ids','types','peer_types'].includes(f.key)).map((f) => c.type === 'time_window' && f.key === 'timezone' ? { ...f, default: undefined, placeholder: '留空使用服务器时区（兼容旧配置）' } : f)
}
function typeFields(c: ConditionConfig) { return (props.descriptors.find((d) => d.type === c.type)?.fields ?? []).filter((f) => f.key === 'types' || f.key === 'peer_types') }
function list(c: ConditionConfig, key: string): string[] { const v = c.config[key]; return Array.isArray(v) ? v.map(String) : typeof v === 'string' ? [v] : [] }
function setKeywords(c: ConditionConfig, text: string) { editedKeywords.add(c); c.config.keywords = text.split(/\r?\n/) }
function cleanKeywords(c: ConditionConfig) { if (editedKeywords.delete(c)) c.config.keywords = [...new Set(list(c, 'keywords').map((s) => s.trim()).filter(Boolean))] }
</script>

<template>
  <div class="condition-editor">
    <NAlert v-if="depth === 0 && serverError" type="error" :show-icon="false">{{ serverError }}</NAlert>
    <p v-if="depth === 0" class="hint">顶层全部满足（AND）。排除条件只阻断当前分支，不是全局黑名单。最多 100 个条件、3 层分组。</p>
    <article v-for="(item, index) in model" :key="index" class="condition-card" :class="{ group: item.type === 'all' || item.type === 'any' }">
      <div class="condition-head">
        <NSelect :value="item.type" :options="options" @update:value="(v: string) => changeType(index, v)" />
        <NCheckbox :checked="item.negate ?? false" @update:checked="(v: boolean) => item.negate = v">取反</NCheckbox>
        <NButton size="small" quaternary type="error" @click="model.splice(index, 1)">移除</NButton>
      </div>
      <ConditionEditor v-if="item.type === 'all' || item.type === 'any'" :model-value="item.children ?? []" :descriptors="descriptors" :depth="depth + 1" :source-options="sources" :sender-options="senders" @update:model-value="(v: ConditionConfig[]) => item.children = v" />
      <template v-else>
        <NFormItem v-if="item.type.startsWith('keyword_')" label="关键词（每行一个）" required>
          <NInput :value="list(item, 'keywords').join('\n')" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" placeholder="广告&#10;推广" @update:value="(v: string) => setKeywords(item, v)" @blur="cleanKeywords(item)" />
        </NFormItem>
        <p v-if="item.type.startsWith('keyword_')" class="hint">{{ list(item, 'keywords').length }} 项；编辑后离开输入框会去重并清除首尾空格。多个词按“任一命中”判断。</p>
        <NFormItem v-if="item.type === 'source'" label="来源（可搜索或填写内部 ID）" required>
          <NSelect :value="list(item, 'source_ids')" :options="sources" multiple filterable tag @update:value="(v: string[]) => item.config.source_ids = v" />
        </NFormItem>
        <NFormItem v-if="item.type === 'sender'" label="发送者 ID（候选来自最近 100 条消息，可手填原始 ID）">
          <NSelect :value="list(item, 'sender_ids')" :options="senders" multiple filterable tag @update:value="(v: string[]) => item.config.sender_ids = v" />
        </NFormItem>
        <p v-if="item.type === 'sender'" class="hint">类型、ID、名称按 AND 组合；同一栏多个值任一即可。不同发送者类型可能同号，建议同时选择类型。</p>
        <ConfigFormRenderer v-model="item.config" :fields="fields(item)" />
        <NFormItem v-for="field in typeFields(item)" :key="field.key" :label="field.label" :required="field.required">
          <NSelect :value="list(item, field.key)" multiple filterable tag :options="(field.options ?? []).map((o) => ({ label: o.label, value: String(o.value) }))" @update:value="(v: string[]) => item.config[field.key] = v" />
        </NFormItem>
        <p v-if="typeFields(item).length" class="hint">内置类型可直接选择；Source 自定义类型可手填，请从消息详情复制准确类型值。</p>
        <template v-if="item.type.startsWith('keyword_') || item.type === 'regex'">
          <NFormItem label="检查范围">
            <NSelect :value="item.config.fields === undefined ? ['text'] : list(item, 'fields')" multiple :options="scopes" @update:value="(v: string[]) => item.config.fields = v" />
          </NFormItem>
          <NFormItem label="文本规范化">
            <NSelect :value="String(item.config.normalize ?? 'none')" :options="normalizations" @update:value="(v: string) => item.config.normalize = v" />
          </NFormItem>
        </template>
        <NFormItem v-if="item.type === 'time_window'" label="判断时间">
          <NSelect :value="String(item.config.time_basis ?? 'sent')" :options="[{ label: '发送时间（缺失时使用接收时间）', value: 'sent' }, { label: '接收时间', value: 'received' }]" @update:value="(v: string) => item.config.time_basis = v" />
        </NFormItem>
        <p v-if="item.type === 'time_window'" class="hint">支持跨午夜；首尾分钟均包含，相同时间只匹配该分钟，不代表全天。</p>
        <p v-if="item.type === 'message_length'" class="hint">按正文 Unicode 字符数判断；最大长度为 0 表示不设上限（兼容旧配置）。</p>
        <p v-if="item.type === 'regex'" class="hint">使用 Go RE2 语法，不支持回溯引用或前后查找；用右侧测试检查表达式。</p>
        <p class="summary">{{ conditionSummary(item) }}</p>
      </template>
      <NAlert v-if="validateCondition(item)" type="warning" :show-icon="false">{{ validateCondition(item) }}</NAlert>
    </article>
    <NButton dashed block @click="add">添加条件</NButton>
  </div>
</template>

<style scoped>
.condition-editor { min-width: 0; display: grid; gap: 12px; }
.condition-card { padding: 14px; border: 1px solid var(--clay-border); border-radius: 10px; background: var(--clay-surface); min-width: 0; }
.condition-card.group { border-left: 3px solid var(--clay-primary); }
.condition-head { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 8px; margin-bottom: 12px; }
.hint, .summary { margin: 4px 0 10px; font-size: 12px; line-height: 1.6; color: var(--clay-text-3); word-break: break-word; }
.summary { color: var(--clay-text-2); }
@media (max-width: 560px) { .condition-card { padding: 10px; } .condition-head { grid-template-columns: minmax(0,1fr) auto; } .condition-head > :first-child { grid-column: 1 / -1; } }
</style>
