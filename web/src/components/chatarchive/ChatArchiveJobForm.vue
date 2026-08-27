<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from 'vue'
import type { Account, ChatExportCreateRequest, SyncedPeer } from '@/types'

const props = defineProps<{
  accounts: Account[]
  peers: SyncedPeer[]
  creating: boolean
  syncing: boolean
}>()

const emit = defineEmits<{
  sync: [accountId: number]
  submit: [request: ChatExportCreateRequest]
}>()

type RangeMode = 'time' | 'id'

const rangeMode = shallowRef<RangeMode>('time')
const syncAttempted = shallowRef(false)
const submitAttempted = shallowRef(false)
const form = reactive({
  accountId: null as number | null,
  peerKey: null as string | null,
  timeRange: null as [number, number] | null,
  fromMessageId: null as number | null,
  toMessageId: null as number | null,
  maxMessages: 50000 as number | null,
  includeMedia: false,
})

const accountOptions = computed(() =>
  props.accounts.map((account) => ({
    label: `${account.name}（${account.status}）`,
    value: account.id,
    disabled: account.status !== 'active',
  })),
)

const peerOptions = computed(() =>
  props.peers.map((peer) => ({
    label: `${peer.name}${peer.username ? ' @' + peer.username : ''} · ${peer.display_type}`,
    value: `${peer.peer_type}:${peer.peer_id}`,
  })),
)

const accountError = computed(() => ((syncAttempted.value || submitAttempted.value) && !form.accountId ? '请选择可用账号' : ''))
const peerError = computed(() => (submitAttempted.value && !form.peerKey ? '请选择要归档的会话' : ''))
const idRangeError = computed(() => {
  if (rangeMode.value !== 'id') return ''
  if (!form.fromMessageId && !form.toMessageId) return submitAttempted.value ? '至少填写一个消息 ID 边界' : ''
  if (form.fromMessageId && form.toMessageId && form.fromMessageId > form.toMessageId) {
    return '起始消息 ID 不能大于结束消息 ID'
  }
  return ''
})

watch(
  () => form.accountId,
  () => {
    form.peerKey = null
    syncAttempted.value = false
  },
)

function syncPeers() {
  syncAttempted.value = true
  if (!form.accountId) return
  emit('sync', form.accountId)
}

function submit() {
  submitAttempted.value = true
  if (!form.accountId || !form.peerKey || idRangeError.value) return

  const [peerType, peerID] = form.peerKey.split(':')
  const request: ChatExportCreateRequest = {
    account_id: form.accountId,
    peer_type: peerType,
    peer_id: Number(peerID),
    max_messages: form.maxMessages || undefined,
    include_media: form.includeMedia,
  }
  if (rangeMode.value === 'time' && form.timeRange) {
    request.from_date = new Date(form.timeRange[0]).toISOString()
    request.to_date = new Date(form.timeRange[1]).toISOString()
  }
  if (rangeMode.value === 'id') {
    request.from_message_id = form.fromMessageId || undefined
    request.to_message_id = form.toMessageId || undefined
  }
  emit('submit', request)
}
</script>

<template>
  <NForm label-placement="top" class="archive-job-form" @submit.prevent="submit">
    <div class="form-grid form-grid-main">
      <NFormItem label="Telegram 账号" required :feedback="accountError" :validation-status="accountError ? 'error' : undefined">
        <NSelect v-model:value="form.accountId" :options="accountOptions" placeholder="选择账号" />
      </NFormItem>
      <NFormItem label="会话" required :feedback="peerError" :validation-status="peerError ? 'error' : undefined">
        <div class="peer-control">
          <NSelect v-model:value="form.peerKey" :options="peerOptions" placeholder="选择会话（需先同步）" filterable />
          <NButton attr-type="button" :loading="syncing" :disabled="!form.accountId" @click="syncPeers">同步会话</NButton>
        </div>
      </NFormItem>
    </div>

    <NFormItem label="归档范围">
      <NRadioGroup v-model:value="rangeMode" size="small">
        <NRadioButton value="time">按时间</NRadioButton>
        <NRadioButton value="id">按消息 ID</NRadioButton>
      </NRadioGroup>
    </NFormItem>

    <div class="form-grid form-grid-range">
      <NFormItem v-if="rangeMode === 'time'" label="时间范围" :show-feedback="false" class="range-field">
        <NDatePicker v-model:value="form.timeRange" type="datetimerange" clearable placeholder="留空则从最新消息开始" />
      </NFormItem>
      <NFormItem
        v-else
        label="消息 ID 闭区间"
        class="range-field"
        :feedback="idRangeError || '消息 ID 只在当前会话内有效，允许单侧边界'"
        :validation-status="idRangeError ? 'error' : undefined"
      >
        <div class="id-range">
          <NInputNumber v-model:value="form.fromMessageId" :min="1" :max="2147483647" placeholder="起始 ID" />
          <span class="range-separator">至</span>
          <NInputNumber v-model:value="form.toMessageId" :min="1" :max="2147483647" placeholder="结束 ID" />
        </div>
      </NFormItem>
      <NFormItem label="条数上限" :show-feedback="false">
        <NInputNumber v-model:value="form.maxMessages" :min="1" :step="1000" />
      </NFormItem>
    </div>

    <div class="form-actions">
      <NCheckbox v-model:checked="form.includeMedia">下载媒体文件</NCheckbox>
      <NButton attr-type="submit" type="primary" :loading="creating">开始归档</NButton>
    </div>
    <NText depth="3" class="form-note">
      默认只记录媒体元信息。开启后会下载符合媒体策略的文件，归档媒体保留到删除该归档为止。
    </NText>
  </NForm>
</template>

<style scoped>
.archive-job-form { display: grid; gap: 8px; }
.form-grid { display: grid; gap: 12px; }
.form-grid-main { grid-template-columns: minmax(220px, 0.8fr) minmax(360px, 1.5fr); }
.form-grid-range { grid-template-columns: minmax(360px, 1fr) 180px; align-items: start; }
.peer-control { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; width: 100%; }
.range-field { min-width: 0; }
.range-field :deep(.n-date-picker) { width: 100%; }
.id-range { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); gap: 8px; align-items: center; width: 100%; }
.range-separator { color: var(--clay-text-muted); }
.form-actions { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.form-note { min-height: 20px; font-size: 12px; }

@media (max-width: 760px) {
  .form-grid-main,
  .form-grid-range { grid-template-columns: 1fr; }
  .peer-control { grid-template-columns: 1fr; }
  .id-range { grid-template-columns: 1fr; }
  .range-separator { display: none; }
}
</style>
