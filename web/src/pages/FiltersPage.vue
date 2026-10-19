<script setup lang="ts">
import { computed, h, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NSpace, NText, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import FilterEditorModal from '@/components/filters/FilterEditorModal.vue'
import FilterTestPanel from '@/components/filters/FilterTestPanel.vue'
import DecisionPanel from '@/components/filters/DecisionPanel.vue'
import ConditionDetails from '@/components/filters/ConditionDetails.vue'
import PageHeader from '@/components/PageHeader.vue'
import ClayIcon from '@/components/ClayIcon.vue'
import { filtersApi, flowsApi } from '@/api/client'
import type { Filter, RuleItemDescriptor, FilterReference, FilterVersion, DecisionRecord } from '@/types'
import { conditionSummary, cloneConditions } from '@/utils/filterConditions'
import { errText } from '@/utils/error'
import { formatDateTime } from '@/utils/datetime'

const message = useMessage()
const dialog = useDialog()
const route = useRoute()
const router = useRouter()
const filters = shallowRef<Filter[]>([])
const conditionDescriptors = shallowRef<RuleItemDescriptor[]>([])
const loading = shallowRef(false)
const showModal = shallowRef(false)
const editing = shallowRef<Filter | null>(null)
const draftSourceRevision = ref<number | undefined>()
const q = ref('')
const usage = ref('all')
const page = ref(1)
const selected = ref<Filter | null>(null)
const versionSnapshot = ref<FilterVersion | null>(null)
const drawer = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const refs = ref<FilterReference[]>([])
const versions = ref<FilterVersion[]>([])
const versionTotal = ref(0)
const records = ref<DecisionRecord[]>([])
const recordTotal = ref(0)
const recordStatus = ref('')
const recordPage = ref(1)
const versionPage = ref(1)
let generation = 0
let recordGeneration = 0
let versionGeneration = 0
const filtered = computed(() => filters.value.filter((f) => {
  const text = `${f.name} ${f.description} ${f.conditions.map(conditionSummary).join(' ')}`.toLowerCase()
  return text.includes(q.value.trim().toLowerCase()) && (usage.value === 'all' || (usage.value === 'unused' ? !f.stats?.references : (f.stats?.errors ?? 0) > 0))
}))
watch([q, usage], () => { page.value = 1 })
watch(() => [route.query.filter_id, route.query.revision], ([value]) => { const id = Number(value); if (id > 0) void openDetail(id) })

async function load() {
  loading.value = true
  try {
    const [fs, meta] = await Promise.all([filtersApi.list(), flowsApi.meta()])
    filters.value = fs
    conditionDescriptors.value = meta.conditions
    if (selected.value) selected.value = fs.find((f) => f.id === selected.value?.id) ?? selected.value
  } catch (e) { message.error('加载失败：' + errText(e)) }
  finally { loading.value = false }
}
async function openDetail(id: number) {
  const seq = ++generation
  recordGeneration++; versionGeneration++
  const revision = Number(route.query.revision)
  drawer.value = true
  detailLoading.value = true
  detailError.value = ''
  refs.value = []; versions.value = []; records.value = []
  versionSnapshot.value = null
  recordPage.value = 1; versionPage.value = 1
  selected.value = filters.value.find((f) => f.id === id) ?? null
  try {
    const [f, rs, vs, ds, snapshot] = await Promise.all([filtersApi.get(id), filtersApi.references(id), filtersApi.versions(id), filtersApi.decisions(id, recordStatus.value), revision > 0 ? filtersApi.version(id, revision) : Promise.resolve(null)])
    if (seq !== generation) return
    selected.value = { ...f, stats: filters.value.find((item) => item.id === id)?.stats }
    refs.value = rs; versions.value = vs.data; versionTotal.value = vs.total
    records.value = ds.data; recordTotal.value = ds.total
    versionSnapshot.value = snapshot
  } catch (e) { if (seq === generation) detailError.value = revision > 0 ? `无法加载执行时 v${revision}：${errText(e)}` : errText(e) }
  finally { if (seq === generation) detailLoading.value = false }
}
function closeDetail() { generation++; drawer.value = false; void router.replace({ query: { ...route.query, filter_id: undefined, revision: undefined } }) }
function selectDetail(row: Filter) { if (Number(route.query.filter_id) === row.id && !route.query.revision) void openDetail(row.id); else void router.replace({ query: { ...route.query, filter_id: String(row.id), revision: undefined } }) }
function openCreate() { draftSourceRevision.value = undefined; editing.value = null; showModal.value = true }
function openEdit(row: Filter) { draftSourceRevision.value = undefined; editing.value = row; showModal.value = true }
function testVersion(version: FilterVersion) {
  if (!selected.value) return
  editing.value = { ...selected.value, name: version.name, description: version.description, conditions: cloneConditions(version.conditions) }
  draftSourceRevision.value = version.revision
  message.info(`已将 v${version.revision} 载入为草稿；测试会与当前保存版本比较，只有明确保存才会生效。`)
  showModal.value = true
}
async function saved() { await load(); if (selected.value && drawer.value) await openDetail(selected.value.id) }
function confirmDelete(row: Filter) {
  dialog.warning({ title: '删除过滤器', content: `确定删除「${row.name}」及其配置版本历史？被引用时不能删除，历史消息决策仍保留版本摘要。`, positiveText: '删除', negativeText: '取消', onPositiveClick: async () => {
    try { await filtersApi.remove(row.id); if (selected.value?.id === row.id) closeDetail(); message.success('已删除'); await load() } catch (e) { message.error(errText(e)) }
  } })
}
async function recordsChanged() {
  if (!selected.value) return
  const id = selected.value.id, seq = generation
  const request = ++recordGeneration
  try { const data = await filtersApi.decisions(id, recordStatus.value, (recordPage.value - 1) * 30); if (seq === generation && request === recordGeneration) { records.value = data.data; recordTotal.value = data.total } }
  catch (e) { if (seq === generation && request === recordGeneration) detailError.value = errText(e) }
}
async function versionsChanged() {
  if (!selected.value) return
  const seq = generation
  const request = ++versionGeneration
  try { const data = await filtersApi.versions(selected.value.id, (versionPage.value - 1) * 30); if (seq === generation && request === versionGeneration) { versions.value = data.data; versionTotal.value = data.total } }
  catch (e) { if (seq === generation && request === versionGeneration) detailError.value = errText(e) }
}
const columns: DataTableColumns<Filter> = [
  { title: '名称 / 版本', key: 'name', width: 180, render: (row) => h(NButton, { text: true, type: 'primary', onClick: () => selectDetail(row) }, { default: () => `${row.name} · v${row.revision ?? 1}` }) },
  { title: '条件摘要（顶层 AND）', key: 'conditions', minWidth: 250, ellipsis: { tooltip: true }, render: (row) => row.conditions.map(conditionSummary).join(' 且 ') },
  { title: '引用', key: 'references', width: 100, render: (row) => h(NText, { depth: row.stats?.references ? 1 : 3 }, { default: () => row.stats?.references ? `${row.stats.references} 处` : '未引用 / 未生效' }) },
  { title: '24h 实时执行：通过 / 拦截 / 异常', key: 'stats', width: 235, render: (row) => `${row.stats?.passed ?? 0} / ${row.stats?.blocked ?? 0} / ${row.stats?.errors ?? 0}` },
  { title: '最近修改', key: 'updated_at', width: 165, render: (row) => formatDateTime(row.updated_at) },
  { title: '操作', key: 'actions', width: 175, render: (row) => h(NSpace, { size: 4 }, { default: () => [
    h(NButton, { size: 'small', secondary: true, onClick: () => openEdit(row) }, { default: () => '编辑 / 测试' }),
    h(NButton, { size: 'small', quaternary: true, type: 'error', onClick: () => confirmDelete(row) }, { default: () => '删除' }),
  ] }) },
]
onMounted(async () => { await load(); const id = Number(route.query.filter_id); if (id > 0) await openDetail(id) })
</script>

<template>
  <NSpace vertical size="large">
    <PageHeader title="过滤器" desc="配置意图、验证效果、追踪每一次判断。只有被 Flow 或 AI 整理引用才会生效。" icon="shield">
      <template #actions><NButton type="primary" @click="openCreate"><template #icon><ClayIcon name="plus" :size="16" /></template>新建过滤器</NButton><NButton secondary :loading="loading" @click="load">刷新</NButton></template>
    </PageHeader>
    <div class="filter-toolbar"><NInput v-model:value="q" clearable placeholder="搜索名称、说明、关键词、正则或条件" /><NSelect v-model:value="usage" :options="[{ label: '全部过滤器', value: 'all' }, { label: '未被引用', value: 'unused' }, { label: '24h 有异常', value: 'error' }]" /></div>
    <NText depth="3">执行统计：近 24 小时实时 Flow 的过滤器执行次数（同一消息经过多个节点可能计多次）；AI 结果在对应运行详情中查看。</NText>
    <NDataTable :loading="loading" :columns="columns" :data="filtered" :bordered="false" :scroll-x="1120" :row-key="(row: Filter) => row.id" :pagination="{ page, pageSize: 20, onChange: (value: number) => page = value }" />
    <FilterEditorModal v-model:show="showModal" :filter="editing" :condition-descriptors="conditionDescriptors" :draft-source-revision="draftSourceRevision" @saved="saved" />
    <NDrawer :show="drawer" :width="'min(900px, 100vw)'" @update:show="(v: boolean) => { if (!v) closeDetail() }">
      <NDrawerContent :title="selected ? `${versionSnapshot?.name ?? selected.name} · ${versionSnapshot ? `执行时 v${versionSnapshot.revision}（当前 v${selected.revision}）` : `v${selected.revision ?? 1}`}` : '过滤器详情'" closable :native-scrollbar="false">
        <NSpin :show="detailLoading">
          <NAlert v-if="detailError" type="error">{{ detailError }}</NAlert>
          <template v-if="selected">
            <NSpace justify="space-between"><NText>{{ (versionSnapshot ? versionSnapshot.description : selected.description) || '未填写用途说明' }}</NText><NButton secondary @click="openEdit(selected)">编辑当前保存版本</NButton></NSpace>
            <NAlert v-if="versionSnapshot" type="info" :show-icon="false">下方配置是执行时的不可变 v{{ versionSnapshot.revision }}；引用位置与最近执行记录展示当前引用及所有版本记录。</NAlert>
            <NTabs type="line">
              <NTabPane name="config" tab="配置详情">
                <NAlert :type="refs.length ? 'info' : 'warning'" :show-icon="false">{{ refs.length ? `被 ${refs.length} 处引用，修改会联动这些位置。` : '尚未被引用，当前不会自动过滤任何消息。' }}</NAlert>
                <ConditionDetails :conditions="versionSnapshot?.conditions ?? selected.conditions" :descriptors="conditionDescriptors" />
                <NText depth="3">顶层 AND；并联分支为 OR。本过滤器不是全局黑名单，不会撤销已入队任务，也不包含图片 OCR 或语义识别。</NText>
              </NTabPane>
              <NTabPane name="refs" :tab="`引用位置 (${refs.length})`">
                <NEmpty v-if="!refs.length" description="没有引用位置" />
                <div v-for="(r, i) in refs" :key="i" class="reference">
                  <RouterLink :to="r.kind === 'flow' ? { name: 'flow', query: { flow_id: String(r.id) } } : { name: 'ai-digests', query: { profile_id: String(r.id) } }">{{ r.kind === 'flow' ? 'Flow' : 'AI 整理' }} · {{ r.name }} {{ r.node_id ? `· 节点 #${r.node_id}` : '' }}</RouterLink>
                  <NTag size="small">{{ r.enabled ? '启用' : '停用' }}</NTag>
                </div>
              </NTabPane>
              <NTabPane name="test" tab="效果测试"><FilterTestPanel :conditions="versionSnapshot?.conditions ?? selected.conditions" :compare-id="selected.id" /></NTabPane>
              <NTabPane name="records" tab="最近执行">
                <NSelect v-model:value="recordStatus" :options="[{ label: '全部处理结果', value: '' }, { label: '已入队', value: 'enqueued' }, { label: '被过滤', value: 'blocked' }, { label: '异常', value: 'error' }, { label: '匹配但未入队', value: 'not_enqueued' }]" @update:value="recordPage = 1; recordsChanged()" />
                <NText depth="3">这里是完整消息处理结果，不是单个过滤器状态；详细判断见节点。决策默认保留 30 天，可在“设置 → 消息与投递记录归档”调整。</NText>
                <NEmpty v-if="!records.length" description="没有记录；旧消息不推断历史过滤结果" />
                <details v-for="record in records" :key="record.id" class="record"><summary>消息 #{{ record.message_id }} · revision {{ record.message_revision }} · {{ record.trigger }} · {{ formatDateTime(record.created_at) }}</summary><RouterLink :to="{ name: 'messages', query: { message_id: String(record.message_id) } }">打开原消息及投递详情</RouterLink><DecisionPanel :evaluation="record.evaluation" /></details>
                <NSpace><NButton :disabled="recordPage <= 1" @click="recordPage--; recordsChanged()">上一页</NButton><NText>{{ recordPage }} / {{ Math.max(1, Math.ceil(recordTotal / 30)) }}</NText><NButton :disabled="recordPage * 30 >= recordTotal" @click="recordPage++; recordsChanged()">下一页</NButton></NSpace>
              </NTabPane>
              <NTabPane name="history" tab="修改历史">
                <details v-for="(v, i) in versions" :key="v.revision" class="record"><summary>v{{ v.revision }} · {{ formatDateTime(v.created_at) }} · {{ v.name }}</summary><p>{{ v.description }}</p><ConditionDetails :conditions="v.conditions" :descriptors="conditionDescriptors" /><NButton size="small" secondary @click="testVersion(v)">载入此版本为草稿 / 对比测试</NButton><p v-if="versions[i + 1]" class="version-diff">与 v{{ versions[i + 1]!.revision }} 相比：{{ JSON.stringify(v.conditions) === JSON.stringify(versions[i + 1]!.conditions) ? '条件不变，仅元信息变化' : '条件已变更，请用效果测试对比当前草稿与保存版本' }}</p></details>
                <NSpace><NButton :disabled="versionPage <= 1" @click="versionPage--; versionsChanged()">上一页</NButton><NText>{{ versionPage }} / {{ Math.max(1, Math.ceil(versionTotal / 30)) }}</NText><NButton :disabled="versionPage * 30 >= versionTotal" @click="versionPage++; versionsChanged()">下一页</NButton></NSpace>
              </NTabPane>
            </NTabs>
          </template>
        </NSpin>
      </NDrawerContent>
    </NDrawer>
  </NSpace>
</template>

<style scoped>
.filter-toolbar { display: grid; grid-template-columns: minmax(0,1fr) 200px; gap: 12px; }
.reference { display: flex; justify-content: space-between; gap: 10px; padding: 12px 0; border-bottom: 1px solid var(--clay-border); }
a { color: var(--clay-primary); } .condition-summary { padding-left: 24px; } .condition-summary li { padding: 7px 0; overflow-wrap: anywhere; }
.record { padding: 12px; border: 1px solid var(--clay-border); border-radius: 8px; margin: 12px 0; }
.record summary { cursor: pointer; font-weight: 600; margin-bottom: 10px; } .record > a { display: block; margin-bottom: 10px; } .version-diff { color: var(--clay-text-3); font-size: 12px; }
@media(max-width: 600px) { .filter-toolbar { grid-template-columns: minmax(0,1fr); } }
</style>
