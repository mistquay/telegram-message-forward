<script setup lang="ts">
import type { Filter, RuleItemDescriptor } from '@/types'
import ConditionDetails from './ConditionDetails.vue'
defineProps<{ ids: number[]; filters: Filter[]; descriptors: RuleItemDescriptor[] }>()
</script>
<template>
  <div class="shared-details">
    <details v-for="id in ids" :key="id">
      <summary>{{ filters.find((f) => f.id === id)?.name ?? `过滤器 #${id}` }} · 当前加载 v{{ filters.find((f) => f.id === id)?.revision ?? '?' }}</summary>
      <ConditionDetails v-if="filters.some((f) => f.id === id)" :conditions="filters.find((f) => f.id === id)!.conditions" :descriptors="descriptors" />
      <NAlert v-else type="error">引用的过滤器不存在，请修复引用；运行时不会静默放行。</NAlert>
    </details>
    <NText depth="3">这里展示当前已加载配置；保存的是引用 ID，实际求值会固定读取当时最新版本。</NText>
  </div>
</template>
<style scoped>
.shared-details { display: grid; gap: 10px; margin: 12px 0; min-width: 0; }
details { padding: 10px; border: 1px solid var(--clay-border); border-radius: 8px; } summary { cursor: pointer; font-weight: 600; overflow-wrap: anywhere; }
</style>
