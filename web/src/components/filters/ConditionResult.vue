<script setup lang="ts">
import type { ConditionResult } from '@/types'
import { conditionNames } from '@/utils/filterConditions'
defineOptions({ name: 'ConditionResultTree' })
defineProps<{ result: ConditionResult }>()
const labels: Record<string, string> = { passed: '通过', blocked: '不通过', error: '执行失败', skipped: '未执行' }
const scopeNames: Record<string, string> = { text: '正文', links: '链接地址', media_names: '附件名称', media_captions: '附件说明' }
const reasons: Record<string, string> = { short_circuit: '前序条件已确定结果，短路未执行', filter_resolution_failed: '共享过滤器不存在、读取失败或配置无效', filter_resolution_aborted: '其他共享过滤器解析失败，本次未执行', condition_execution_failed: '条件执行失败', stop_on_match: '前一个 Flow 命中即停，本条件未执行', path_not_reached: '路径没有到达此节点，本条件未执行' }
</script>

<template>
  <div class="result-node" :class="result.status">
    <div class="result-head">
      <NTag size="small" :type="result.status === 'passed' ? 'success' : result.status === 'error' ? 'error' : result.status === 'blocked' ? 'warning' : 'default'">{{ labels[result.status] ?? result.status }}</NTag>
      <strong>{{ result.negate ? '取反 · ' : '' }}{{ conditionNames[result.type] ?? result.type }}</strong>
      <span v-if="result.field">{{ result.field.split(',').map((f) => scopeNames[f] ?? f).join('、') }}</span>
      <span v-if="result.duration_us">{{ result.duration_us }} μs</span>
    </div>
    <p v-if="result.evidence">实际命中：<mark>{{ result.evidence }}</mark></p>
    <p v-if="result.reason">原因：{{ reasons[result.reason] ?? result.reason }}</p>
    <div v-if="result.children?.length" class="children">
      <ConditionResultTree v-for="(child, i) in result.children" :key="i" :result="child" />
    </div>
  </div>
</template>

<style scoped>
.result-node { border-left: 2px solid var(--clay-border); padding: 8px 0 8px 10px; min-width: 0; font-size: 12px; }
.result-head { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.result-head span, p { color: var(--clay-text-3); }
.passed { border-color: #38a18a; } .blocked { border-color: #d6a145; } .error { border-color: #d14f47; }
.children { margin: 8px 0 0 8px; } p { margin: 6px 0; overflow-wrap: anywhere; } mark { background: #fff0bc; color: #77500b; padding: 1px 4px; border-radius: 3px; }
</style>
