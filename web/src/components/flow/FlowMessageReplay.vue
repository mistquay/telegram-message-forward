<script setup lang="ts">
import { computed, ref } from 'vue'
import { flowsApi } from '@/api/client'
import type { Flow } from '@/types'
import FlowPreviewPanel from './FlowPreviewPanel.vue'
import { errText } from '@/utils/error'
const props = defineProps<{ messageId: number; sourceId: number }>()
const open = ref(false)
const flows = ref<Flow[]>([])
const selected = ref<number | null>(null)
const error = ref('')
const flow = computed(() => flows.value.find((f) => f.id === selected.value))
async function load() {
  open.value = true; error.value = ''
  try { flows.value = (await flowsApi.list()).filter((f) => f.nodes.some((n) => n.type === 'source' && n.ref_id === props.sourceId)); selected.value = flows.value[0]?.id ?? null }
  catch (e) { error.value = errText(e) }
}
</script>
<template>
  <NButton secondary @click="load">按当前配置重新评估（只读）</NButton>
  <NModal v-model:show="open" preset="card" title="当前配置重评 · 不修改历史记录" :style="{ width: 'min(900px, calc(100vw - 24px))' }">
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSelect v-model:value="selected" :options="flows.map((f) => ({ label: f.name, value: f.id }))" placeholder="选择包含这个来源的 Flow" class="flow-select" />
    <NEmpty v-if="!flow" description="没有适用的 Flow" /><FlowPreviewPanel v-else :key="`${flow.id}-${messageId}`" :flow="flow" :initial-message-id="messageId" />
  </NModal>
</template>
<style scoped>.flow-select { margin-bottom: 16px; }</style>
