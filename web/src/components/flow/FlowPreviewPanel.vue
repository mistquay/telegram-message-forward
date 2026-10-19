<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { flowsApi } from '@/api/client'
import type { Flow, FlowPreview, Source } from '@/types'
import DecisionPanel from '@/components/filters/DecisionPanel.vue'
import { errText } from '@/utils/error'

const props = defineProps<{ flow: Flow; initialMessageId?: number; sources?: Source[] }>()
const fullChain = ref(true)
const edit = ref(false)
const useReal = ref(Boolean(props.initialMessageId))
const messageId = ref<number | null>(props.initialMessageId ?? null)
const sample = reactive({ source_id: props.flow.nodes.find((n) => n.type === 'source')?.ref_id ?? 0, text: '', message_type: 'text', sender_id: 0, sender_name: '', sender_peer_type: 'user' })
const sent = ref<number | null>(null)
const received = ref<number | null>(null)
const media = ref('')
const links = ref('')
const output = ref<FlowPreview | null>(null)
const error = ref('')
const loading = ref(false)
let generation = 0
const sourceOptions = computed(() => props.sources?.map((s) => ({ label: s.name, value: s.id })) ?? props.flow.nodes.filter((n) => n.type === 'source' && n.ref_id).map((n) => ({ label: `来源 #${n.ref_id}`, value: n.ref_id! })))
watch(useReal, (value) => { if (!value) edit.value = false })
watch(() => [props.flow, fullChain.value, edit.value, useReal.value, messageId.value, sample, sent.value, received.value, media.value, links.value], () => { generation++; output.value = null; error.value = '' }, { deep: true })
async function run() {
  const seq = ++generation
  loading.value = true; error.value = ''
  try {
    const result = await flowsApi.preview({ flow: props.flow, flow_id: props.flow.id > 0 ? props.flow.id : undefined, full_chain: fullChain.value, edit: edit.value,
      ...(useReal.value ? { message_id: messageId.value ?? 0 } : { message: { ...sample, links: links.value.split(/\r?\n/).filter(Boolean).map((url) => ({ url })), media: media.value ? [{ type: media.value }] : [], ...(sent.value ? { sent_at: new Date(sent.value).toISOString() } : {}), ...(received.value ? { received_at: new Date(received.value).toISOString() } : {}) } }),
    })
    if (seq === generation) output.value = result
  } catch (e) { if (seq === generation) error.value = errText(e) }
  finally { loading.value = false }
}
const targetLabels: Record<string, string> = { would_enqueue: '模拟：路由允许（实际入队仍受幂等约束）', skipped_sink_disabled: '不会入队：渠道停用', skipped_no_historical_route: '不会补发：不是原历史路由' }
</script>

<template>
  <div class="flow-preview">
    <NAlert type="info" :show-icon="false">只读预演：不保存草稿、不创建任务、不发消息、不调用 AI。结果基于当前配置，不是历史处理事实。</NAlert>
    <NSpace><NCheckbox v-model:checked="fullChain">包含同源其他 Flow（优先级 / 命中即停）</NCheckbox><NCheckbox v-model:checked="useReal">使用已采集消息</NCheckbox><NCheckbox v-model:checked="edit" :disabled="!useReal">模拟编辑补发交集</NCheckbox></NSpace>
    <NText v-if="edit" depth="3">编辑模拟假设当前内容是一条新编辑版本，只检查过滤和历史目标交集；不模拟 Telegram 内容是否变化及队列幂等，不会补发消息。</NText>
    <NInputNumber v-if="useReal" v-model:value="messageId" :min="1" placeholder="消息中心内部 ID" />
    <template v-else>
      <NSelect v-model:value="sample.source_id" :options="sourceOptions" placeholder="选择来源" />
      <NInput v-model:value="sample.text" type="textarea" :autosize="{ minRows: 4, maxRows: 12 }" placeholder="样例消息正文" />
      <details><summary>消息类型、发送者、时间与链接</summary><div class="sample-fields">
        <NSelect v-model:value="sample.message_type" :options="['text','photo','image','document','video','audio','voice'].map((v) => ({ label: v, value: v }))" />
        <NInputNumber v-model:value="sample.sender_id" :min="0" placeholder="发送者 ID" /><NInput v-model:value="sample.sender_name" placeholder="发送者名称" />
        <NSelect v-model:value="sample.sender_peer_type" :options="['user','chat','channel','rss','webhook'].map((v) => ({ label: v, value: v }))" />
        <NDatePicker v-model:value="sent" type="datetime" clearable placeholder="发送时间" /><NDatePicker v-model:value="received" type="datetime" clearable placeholder="接收时间" />
        <NSelect v-model:value="media" :options="[{ label: '无附件', value: '' }, ...['photo','image','document','video','audio','voice'].map((v) => ({ label: v, value: v }))]" />
        <NInput v-model:value="links" type="textarea" placeholder="链接地址，每行一个" />
      </div></details>
    </template>
    <NButton type="primary" :loading="loading" @click="run">预演当前配置</NButton>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <template v-if="output">
      <DecisionPanel :evaluation="output.evaluation" />
      <NEmpty v-if="!output.targets.length" description="没有匹配目标，查看上方条件判断原因" size="small" />
      <section v-for="(target, i) in output.targets" :key="i" class="target-output"><strong>Flow #{{ target.flow_id }} / 节点 #{{ target.node_id }} → 渠道 #{{ target.sink_id }}</strong><NTag size="small">{{ targetLabels[target.status] ?? target.status }}</NTag><pre>{{ target.text || '[无正文]' }}</pre><NText v-if="target.media?.length" depth="3">附件 {{ target.media.length }} 个</NText></section>
    </template>
  </div>
</template>

<style scoped>
.flow-preview { display: grid; gap: 14px; min-width: 0; }
.sample-fields { display: grid; gap: 10px; padding-top: 12px; }
summary { cursor: pointer; font-weight: 600; } .target-output { display: grid; gap: 8px; border: 1px solid var(--clay-border); padding: 12px; border-radius: 8px; min-width: 0; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 260px; overflow: auto; font-family: inherit; margin: 0; }
</style>
