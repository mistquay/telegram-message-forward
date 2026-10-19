<script setup lang="ts">
import type { Evaluation, NodeDecision } from '@/types'
import ConditionResultTree from './ConditionResult.vue'
import ConditionDetails from './ConditionDetails.vue'
defineProps<{ evaluation?: Evaluation; node?: NodeDecision }>()
const statuses: Record<string, string> = { passed: '通过', blocked: '被过滤', error: '处理异常', skipped: '未执行', no_flow: '无适用 Flow', matched: '已匹配目标', enqueued: '已入队（非投递成功）', not_enqueued: '未入队', deduplicated: '任务已存在（未重复入队）', rolled_back: '事务已回滚（未入队）', commit_unknown: '提交结果未确认，请核对关联任务' }
const reasons: Record<string, string> = { dispatch_not_started: '投递事务未开始，没有创建新任务', sink_lookup_failed: '读取目标渠道状态失败', stop_on_match: '前一个 Flow 命中即停', path_not_reached: '路径未到达（上游拦截或来源不匹配）', filter_resolution_failed: '共享过滤器解析失败，本次不放行', evaluation_failed: '条件求值失败', enqueue_failed: '创建任务失败', target_deduplicated: '同一目标节点已处理', sink_disabled: '目标渠道已停用', edit_route_not_matched: '编辑内容未通过当前过滤或原路由已变更', dispatch_rolled_back: '任务或决策保存失败，事务已回滚，没有创建新投递任务', dispatch_commit_unknown: '数据库未确认事务提交结果；不能据此判断消息是否入队或送达，请核对任务' }
</script>

<template>
  <div class="decisions">
    <template v-if="evaluation">
      <NAlert v-if="evaluation.truncated" type="warning" :show-icon="false">执行详情超过大小上限，部分 Flow、节点、条件和配置快照已省略；处理结果与过滤器执行统计仍按完整执行结果记录。</NAlert>
      <NSpace><NTag :type="evaluation.status === 'error' ? 'error' : 'info'">{{ statuses[evaluation.status] ?? evaluation.status }}</NTag><NText depth="3">求值 {{ evaluation.duration_us / 1000 }} ms</NText></NSpace>
      <NAlert v-if="evaluation.reason" :type="evaluation.status === 'error' ? 'error' : 'info'" :show-icon="false">{{ reasons[evaluation.reason] ?? evaluation.reason }}</NAlert>
      <details v-for="flow in evaluation.flows" :key="flow.id" open>
        <summary>{{ flow.name }} · {{ statuses[flow.status] ?? flow.status }} <small>{{ flow.reason ? (reasons[flow.reason] ?? flow.reason) : '' }}</small></summary>
        <p class="version">{{ flow.version.startsWith('draft') ? '草稿指纹' : '保存版本' }} {{ flow.version }}</p>
        <div v-for="(step, i) in flow.nodes" :key="i" class="step">
          <strong>{{ step.from_node_id ? `#${step.from_node_id} → ` : '' }}{{ step.type }} #{{ step.node_id }} · {{ statuses[step.status] ?? step.status }}</strong>
          <p v-if="step.reason">{{ reasons[step.reason] ?? step.reason }}</p>
          <DecisionPanel :node="step" />
        </div>
      </details>
      <div v-for="(target, i) in evaluation.targets" :key="i" class="target">
        渠道 #{{ target.sink_id }} · {{ statuses[target.status] ?? target.status }}
        <RouterLink v-if="target.task_id" :to="{ name: 'deliveries', query: { task_id: String(target.task_id) } }">任务 #{{ target.task_id }}</RouterLink>
        <span v-if="target.reason"> · {{ reasons[target.reason] ?? target.reason }}</span>
      </div>
    </template>
    <template v-if="node">
      <details v-if="node.config_snapshot?.length"><summary>当时的内联条件配置</summary><ConditionDetails :conditions="node.config_snapshot" :descriptors="[]" /></details>
      <div v-for="(filter, i) in node.filters ?? []" :key="i">
        <RouterLink :to="{ name: 'filters', query: { filter_id: String(filter.id), revision: filter.revision ? String(filter.revision) : undefined } }">{{ filter.name || `过滤器 #${filter.id}` }} · v{{ filter.revision || '?' }}</RouterLink>
        <ConditionResultTree :result="filter.result" />
      </div>
      <ConditionResultTree v-if="node.conditions" :result="node.conditions" />
    </template>
  </div>
</template>

<style scoped>
.decisions { display: grid; gap: 10px; min-width: 0; font-size: 12px; }
details { padding: 12px; border: 1px solid var(--clay-border); border-radius: 8px; }
summary { cursor: pointer; font-weight: 700; overflow-wrap: anywhere; }
.step { margin-top: 12px; padding-top: 8px; border-top: 1px solid var(--clay-border); }
.version, .step p { color: var(--clay-text-3); margin: 5px 0; }
a { color: var(--clay-primary); } .target { padding: 8px; background: var(--clay-surface-2); border-radius: 6px; }
</style>
