<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import ConditionEditor from './ConditionEditor.vue'
import FilterTestPanel from './FilterTestPanel.vue'
import { filtersApi } from '@/api/client'
import type { ConditionConfig, Filter, RuleItemDescriptor } from '@/types'
import { errText } from '@/utils/error'
import { cloneConditions } from '@/utils/filterConditions'

const show = defineModel<boolean>('show', { required: true })
const props = defineProps<{ filter?: Filter | null; conditionDescriptors: RuleItemDescriptor[]; draftSourceRevision?: number }>()
const emit = defineEmits<{ saved: [] }>()
const message = useMessage()
const dialog = useDialog()
const form = reactive({ name: '', description: '', conditions: [] as ConditionConfig[] })
const saving = ref(false)
const error = ref('')
const baseline = ref('')
const tab = ref('config')
const compact = ref(false)
let mediaQuery: MediaQueryList | null = null
function updateLayout() { compact.value = mediaQuery?.matches ?? false }
onMounted(() => { mediaQuery = window.matchMedia('(max-width: 900px)'); updateLayout(); mediaQuery.addEventListener('change', updateLayout) })
onBeforeUnmount(() => mediaQuery?.removeEventListener('change', updateLayout))
const dirty = computed(() => Boolean(props.draftSourceRevision) || JSON.stringify(form) !== baseline.value)

watch(() => [show.value, props.filter] as const, () => {
  if (!show.value) return
  form.name = props.filter?.name ?? ''
  form.description = props.filter?.description ?? ''
  form.conditions = cloneConditions(props.filter?.conditions ?? [])
  error.value = ''
  baseline.value = JSON.stringify(form)
}, { immediate: true })

function close(): void {
  if (!dirty.value) { show.value = false; return }
  dialog.warning({ title: '放弃未保存的修改？', content: '关闭后当前草稿将丢失。测试不会保存配置。', positiveText: '放弃修改', negativeText: '继续编辑', onPositiveClick: () => { show.value = false } })
}
async function submit(): Promise<void> {
  if (!form.name.trim() || !form.conditions.length) { error.value = '请填写名称并至少添加一个条件'; return }
  saving.value = true
  error.value = ''
  try {
    if (props.filter?.id) {
      const refs = await filtersApi.references(props.filter.id)
      if (refs.length) {
        const confirmed = await new Promise<boolean>((resolve) => dialog.warning({ title: '修改共享过滤器', content: `将影响 ${refs.filter((r) => r.kind === 'flow').length} 个 Flow 节点和 ${refs.filter((r) => r.kind === 'ai_digest').length} 个 AI 整理配置。建议先测试并比较新旧结果；已入队任务不会被撤销。`, positiveText: '确认保存', negativeText: '返回检查', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) }))
        if (!confirmed) return
      }
    }
    const body = { name: form.name.trim(), description: form.description.trim(), conditions: form.conditions, revision: props.filter?.revision }
    if (props.filter?.id) await filtersApi.update(props.filter.id, body)
    else await filtersApi.create(body)
    message.success('已保存过滤器新版本')
    show.value = false
    emit('saved')
  } catch (e) { error.value = errText(e) }
  finally { saving.value = false }
}
</script>

<template>
  <NModal :show="show" preset="card" :title="filter ? `编辑过滤器 · v${filter.revision ?? 1}` : '新建过滤器'" :style="{ width: 'min(1220px, calc(100vw - 24px))' }" :mask-closable="false" :close-on-esc="false" @update:show="(v: boolean) => { if (!v) close() }">
    <NAlert v-if="draftSourceRevision" type="info" class="save-error" :show-icon="false">已将历史 v{{ draftSourceRevision }} 载入草稿；当前保存版本仍为 v{{ filter?.revision }}，只有保存新版本才会生效，旧版本不会被覆盖。</NAlert>
    <NAlert v-if="error" type="error" class="save-error" :show-icon="false">{{ error }}</NAlert>
    <div class="editor-workspace">
      <section class="configuration">
        <NForm label-placement="top">
          <NFormItem label="名称" required><NInput v-model:value="form.name" placeholder="例如：排除广告与推广" maxlength="100" /></NFormItem>
          <NFormItem label="用途说明"><NInput v-model:value="form.description" placeholder="说明匹配范围和使用场景" /></NFormItem>
        </NForm>
        <ConditionEditor v-if="!compact" v-model="form.conditions" :descriptors="conditionDescriptors" />
        <NTabs v-else v-model:value="tab" type="line">
          <NTabPane name="config" tab="条件配置"><ConditionEditor v-model="form.conditions" :descriptors="conditionDescriptors" /></NTabPane>
          <NTabPane name="test" tab="效果测试"><FilterTestPanel :conditions="form.conditions" :compare-id="filter?.id" /></NTabPane>
        </NTabs>
      </section>
      <aside v-if="!compact" class="desktop-test"><FilterTestPanel :conditions="form.conditions" :compare-id="filter?.id" /></aside>
    </div>
    <template #footer><NSpace justify="end"><NText v-if="dirty" depth="3">有未保存修改</NText><NButton @click="close">取消</NButton><NButton type="primary" :loading="saving" @click="submit">保存新版本</NButton></NSpace></template>
  </NModal>
</template>

<style scoped>
.editor-workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, .95fr); gap: 24px; max-height: 72vh; overflow-y: auto; }
.configuration, .desktop-test { min-width: 0; } .desktop-test { border-left: 1px solid var(--clay-border); padding-left: 20px; }
.save-error { margin-bottom: 16px; }
@media (max-width: 900px) { .editor-workspace { grid-template-columns: minmax(0,1fr); } .desktop-test { display: none; } }
</style>
