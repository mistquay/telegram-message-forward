<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { filtersApi, sourcesApi, messagesApi } from '@/api/client'
import type { ConditionConfig, FilterPreview, PreviewMessage, MessageItem } from '@/types'
import ConditionResultTree from './ConditionResult.vue'
import { errText } from '@/utils/error'

const props = defineProps<{ conditions: ConditionConfig[]; compareId?: number }>()
const mode = ref('sample')
const sample = reactive({ source_id: 0, text: '', sender_id: 0, sender_name: '', sender_peer_type: 'user', message_type: 'text' })
const messageId = ref<number | null>(null)
const sourceId = ref<number | null>(null)
const sourceOptions = ref<{ label: string; value: number }[]>([])
const sent = ref<number | null>(null)
const received = ref<number | null>(null)
const range = ref<[number, number] | null>([Date.now() - 86400000, Date.now()])
const keyword = ref('')
const messages = ref<MessageItem[]>([])
const searchLoading = ref(false)
const busy = ref(false)
const error = ref('')
const result = ref<FilterPreview | null>(null)
const links = ref('')
const mediaType = ref<string | null>(null)
const mediaName = ref('')
const mediaCaption = ref('')
let generation = 0
const messageOptions = computed(() => messages.value.map((m) => ({ label: `#${m.id} · ${m.source_name} · ${(m.text || m.message_type).slice(0, 90)}`, value: m.id })))

onMounted(async () => { try { sourceOptions.value = (await sourcesApi.list()).map((s) => ({ label: `${s.name} #${s.id}`, value: s.id })) } catch { error.value = '来源列表加载失败，可以手工填写消息 ID 测试' } })
watch(() => [props.conditions, mode.value, sample, messageId.value, sourceId.value, range.value, links.value, mediaType.value, mediaName.value, mediaCaption.value, sent.value, received.value], () => { generation++; result.value = null; error.value = '' }, { deep: true })

async function searchMessages() {
  searchLoading.value = true
  try { messages.value = (await messagesApi.page({ q: keyword.value, source_id: sourceId.value || undefined, limit: 30 })).data }
  catch (e) { error.value = errText(e) }
  finally { searchLoading.value = false }
}

async function run(next = false) {
  const seq = ++generation
  busy.value = true
  error.value = ''
  try {
    const message: PreviewMessage = { ...sample, links: links.value.split(/\r?\n/).filter(Boolean).map((url) => ({ url })), media: mediaType.value ? [{ type: mediaType.value, file_name: mediaName.value, caption: mediaCaption.value }] : [], ...(sent.value ? { sent_at: new Date(sent.value).toISOString() } : {}), ...(received.value ? { received_at: new Date(received.value).toISOString() } : {}) }
    const out = await filtersApi.preview({ conditions: props.conditions, compare_id: props.compareId,
      ...(mode.value === 'sample' ? { message } : mode.value === 'message' ? { message_id: messageId.value ?? 0 }
        : { source_id: sourceId.value ?? 0, from: range.value ? new Date(range.value[0]).toISOString() : undefined, to: range.value ? new Date(range.value[1]).toISOString() : undefined, limit: 50, before_id: next ? result.value?.next_cursor : undefined }),
    })
    if (seq === generation) result.value = out
  } catch (e) { if (seq === generation) { result.value = null; error.value = errText(e) } }
  finally { busy.value = false }
}
</script>

<template>
  <div class="test-panel">
    <header><strong>验证当前配置</strong><NTag size="small" type="info">只读 · 不保存 · 不发送 · 不调用 AI</NTag></header>
    <NRadioGroup v-model:value="mode" size="small"><NRadioButton value="sample">手工样例</NRadioButton><NRadioButton value="message">真实消息</NRadioButton><NRadioButton value="history">历史样本</NRadioButton></NRadioGroup>
    <template v-if="mode === 'sample'">
      <NInput v-model:value="sample.text" type="textarea" placeholder="输入消息正文；排除词会高亮显示在判断结果中" :autosize="{ minRows: 5, maxRows: 12 }" />
      <details><summary>来源、发送者、时间与附件</summary>
        <div class="advanced">
          <NFormItem label="来源"><NSelect v-model:value="sample.source_id" :options="sourceOptions" filterable /></NFormItem>
          <NFormItem label="发送者 ID"><NInputNumber v-model:value="sample.sender_id" :min="0" /></NFormItem>
          <NFormItem label="发送者名称"><NInput v-model:value="sample.sender_name" /></NFormItem>
          <NFormItem label="发送者类型"><NSelect v-model:value="sample.sender_peer_type" :options="['user','chat','channel','rss','webhook'].map((v) => ({ label: v, value: v }))" /></NFormItem>
          <NFormItem label="消息类型"><NSelect v-model:value="sample.message_type" :options="['text','photo','image','document','video','audio','voice','geo','contact','poll','media'].map((v) => ({ label: v, value: v }))" /></NFormItem>
          <NFormItem label="发送时间"><NDatePicker v-model:value="sent" type="datetime" clearable /></NFormItem>
          <NFormItem label="接收时间（不填用当前时间）"><NDatePicker v-model:value="received" type="datetime" clearable /></NFormItem>
          <NFormItem label="附件类型"><NSelect v-model:value="mediaType" clearable :options="['photo','image','document','video','audio','voice'].map((v) => ({ label: v, value: v }))" /></NFormItem>
          <NInput v-model:value="mediaName" placeholder="附件文件名" /><NInput v-model:value="mediaCaption" placeholder="附件说明" />
          <NInput v-model:value="links" type="textarea" placeholder="链接地址，每行一个（只匹配，不访问链接）" />
        </div>
      </details>
    </template>
    <template v-else-if="mode === 'message'">
      <NSpace><NInput v-model:value="keyword" placeholder="按正文查找已采集消息" @keydown.enter="searchMessages" /><NButton :loading="searchLoading" @click="searchMessages">搜索</NButton></NSpace>
      <NSelect v-model:value="messageId" :options="messageOptions" filterable placeholder="选择搜索结果" />
      <NInputNumber v-model:value="messageId" :min="1" placeholder="也可直接填写消息中心内部 ID" />
    </template>
    <template v-else>
      <NSelect v-model:value="sourceId" :options="sourceOptions" filterable placeholder="选择一个来源" />
      <NDatePicker v-model:value="range" type="datetimerange" clearable />
      <NText depth="3">最多 31 天，每页 50 条，按实际接收时间选取；统计仅针对本页样本，不代表整个时间窗口。</NText>
    </template>
    <NButton type="primary" :loading="busy" :disabled="!conditions.length" @click="run(false)">测试当前配置{{ compareId ? '并对比已保存版本' : '' }}</NButton>
    <NAlert v-if="error" type="error" :show-icon="false">{{ error }}</NAlert>
    <template v-if="result">
      <NSpace><NTag type="success">通过 {{ result.passed }}</NTag><NTag type="warning">不通过 {{ result.blocked }}</NTag><NTag v-if="compareId">结果变化 {{ result.changed }} · 对比 v{{ result.previous_revision }}</NTag></NSpace>
      <NEmpty v-if="!result.items.length" description="范围内没有样本" size="small" />
      <details v-for="(item, i) in result.items" :key="i" open class="sample-result">
        <summary>{{ item.message_id ? `消息 #${item.message_id}` : '手工样例' }} {{ item.changed ? '· 结果变化' : '' }}</summary>
        <p class="sample-text">{{ item.message_text || '[无正文]' }}</p>
        <NText v-if="item.previous" depth="3">保存版本：{{ item.previous.status === 'passed' ? '通过' : '不通过' }} → 当前草稿：{{ item.result.status === 'passed' ? '通过' : '不通过' }}</NText>
        <ConditionResultTree :result="item.result" />
      </details>
      <NButton v-if="result.has_more" :loading="busy" @click="run(true)">检查下一页样本</NButton>
    </template>
  </div>
</template>

<style scoped>
.test-panel { display: grid; gap: 12px; min-width: 0; }
header { display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap; }
summary { cursor: pointer; font-weight: 600; } .advanced { display: grid; gap: 8px; padding-top: 12px; }
.sample-result { padding: 12px; border: 1px solid var(--clay-border); border-radius: 8px; }
.sample-text { max-height: 180px; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; color: var(--clay-text-2); }
</style>
