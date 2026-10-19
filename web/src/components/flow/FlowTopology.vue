<script setup lang="ts">
import { computed } from 'vue'
import type { Flow, Filter, Source, Sink, FlowNode } from '@/types'
import { conditionSummary } from '@/utils/filterConditions'
const props = defineProps<{ flow: Flow; filters: Filter[]; sources: Source[]; sinks: Sink[] }>()
const positioned = computed(() => {
  const depths = new Map<number, number>()
  for (let i = 0; i < props.flow.nodes.length; i++) for (const e of props.flow.edges) depths.set(e.to_node_id, Math.max(depths.get(e.to_node_id) ?? 0, (depths.get(e.from_node_id) ?? 0) + 1))
  const lanes = new Map<number, number>()
  return props.flow.nodes.map((n) => { const depth = depths.get(n.id) ?? 0; const lane = lanes.get(depth) ?? 0; lanes.set(depth, lane + 1); return { ...n, x: 20 + depth * 270, y: 20 + lane * 115 } })
})
const width = computed(() => Math.max(600, ...positioned.value.map((n) => n.x + 250)))
const height = computed(() => Math.max(170, ...positioned.value.map((n) => n.y + 95)))
function label(n: FlowNode) {
  if (n.type === 'source') return props.sources.find((s) => s.id === n.ref_id)?.name ?? `来源 #${n.ref_id}`
  if (n.type === 'target') return props.sinks.find((s) => s.id === n.ref_id)?.name ?? `渠道 #${n.ref_id}`
  if (n.type === 'filter') return (n.config.filter_ids?.length ? n.config.filter_ids.map((id) => props.filters.find((f) => f.id === id)?.name ?? `#${id}`).join(' AND ') : (n.config.conditions ?? []).map(conditionSummary).join(' 且 ')) || '空过滤节点'
  return n.config.processors?.map((p) => p.type).join(' → ') || '处理节点'
}
function point(id: number) { return positioned.value.find((n) => n.id === id) }
</script>
<template>
  <div class="topology">
    <p>真实执行图：实线表示消息路径；串联过滤为 AND，分支汇合为 OR。同一目标只处理一次。共享过滤器是节点内条件引用，不是独立执行路径。</p>
    <div class="graph-scroll"><svg :width="width" :height="height" role="img" aria-label="真实 Flow 执行结构">
      <defs><marker id="flow-topology-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#70849a" /></marker></defs>
      <template v-for="edge in flow.edges" :key="`${edge.from_node_id}-${edge.to_node_id}`"><path v-if="point(edge.from_node_id) && point(edge.to_node_id)" :d="`M ${point(edge.from_node_id)!.x + 230} ${point(edge.from_node_id)!.y + 35} C ${point(edge.from_node_id)!.x + 250} ${point(edge.from_node_id)!.y + 35}, ${point(edge.to_node_id)!.x - 20} ${point(edge.to_node_id)!.y + 35}, ${point(edge.to_node_id)!.x} ${point(edge.to_node_id)!.y + 35}`" fill="none" stroke="#70849a" stroke-width="1.5" marker-end="url(#flow-topology-arrow)" /></template>
      <g v-for="n in positioned" :key="n.id" :transform="`translate(${n.x},${n.y})`"><title>{{ label(n) }}</title><rect width="230" height="72" rx="8" :class="n.type" /><text x="12" y="23" class="kind">{{ n.type }} #{{ n.id }} {{ flow.edges.filter((e) => e.to_node_id === n.id).length > 1 ? '· 汇合 OR' : '' }}</text><text x="12" y="48">{{ label(n).slice(0, 23) }}{{ label(n).length > 23 ? '…' : '' }}</text></g>
    </svg></div>
    <ol><li v-for="n in flow.nodes.filter((node) => node.type === 'filter')" :key="n.id"><strong>过滤节点 #{{ n.id }}</strong>：{{ label(n) }}<div v-for="id in n.config.filter_ids ?? []" :key="id">{{ (filters.find((f) => f.id === id)?.conditions ?? []).map(conditionSummary).join(' 且 ') }}</div></li></ol>
  </div>
</template>
<style scoped>
.topology { min-width: 0; } p { color: var(--clay-text-3); line-height: 1.6; font-size: 12px; }
.graph-scroll { max-width: 100%; overflow: auto; border: 1px solid var(--clay-border); border-radius: 8px; background: var(--clay-surface-2); }
rect { fill: var(--clay-surface); stroke: #70849a; } rect.filter { stroke: #3b82f6; } rect.source { stroke: #2b9c88; } rect.target { stroke: #d0a045; }
text { fill: var(--clay-text); font-size: 12px; } text.kind { fill: var(--clay-text-3); font-size: 11px; }
ol { padding-left: 24px; font-size: 12px; } li { padding: 6px 0; overflow-wrap: anywhere; }
</style>
