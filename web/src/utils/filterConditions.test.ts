import test from 'node:test'
import assert from 'node:assert/strict'
import { conditionSummary, newCondition, validateCondition, cloneConditions } from './filterConditions.ts'
import { flowToLinearFlow } from '../api/client.ts'
import type { Flow, ConditionConfig } from '../types/index.ts'

test('condition summaries preserve groups, negation and scope', () => {
  const summary = conditionSummary({ type: 'any', config: {}, children: [
    { type: 'keyword_contains', config: { keywords: ['go'], fields: ['links'] } },
    { type: 'keyword_contains', config: { keywords: ['spam'] }, negate: true },
  ] })
  assert.match(summary, /go/)
  assert.match(summary, /links/)
  assert.match(summary, /或/)
  assert.match(summary, /取反/)
})

test('invalid IDs, empty sender and impossible ranges are visible', () => {
  assert.ok(validateCondition({ type: 'source', config: { source_ids: ['not-an-id'] } }))
  assert.ok(validateCondition({ type: 'sender', config: {} }))
  assert.ok(validateCondition({ type: 'message_length', config: { min: 10, max: 2 } }))
  assert.equal(validateCondition({ type: 'source', config: { source_ids: ['123'] } }), '')
})

test('new defaults are detached from descriptor data', () => {
  const descriptors = [{ type: 'keyword_contains', label: 'contains', fields: [{ key: 'keywords', type: 'string_list' as const, label: 'keywords', default: ['go'] }] }]
  const a = newCondition('keyword_contains', descriptors)
  ;(a.config.keywords as string[]).push('changed')
  assert.deepEqual(descriptors[0].fields[0].default, ['go'])
})

test('legacy omitted configs and nested negation can be edited without losing structure', () => {
  const input = [{ type: 'any', children: [{ type: 'has_media', negate: true }] }] as ConditionConfig[]
  const copy = cloneConditions(input)
  assert.deepEqual(copy[0].config, {})
  assert.deepEqual(copy[0].children?.[0].config, {})
  assert.equal(copy[0].children?.[0].negate, true)
  assert.equal(input[0].config, undefined)
})

test('resource overview never hides inline conditions beside shared references', () => {
  const flow: Flow = { id: 1, name: 'mixed branches', enabled: true, priority: 1, stop_on_match: false, created_at: '', updated_at: '', edges: [], nodes: [
    { id: 1, type: 'filter', pos_x: 0, pos_y: 0, config: { filter_ids: [7] } },
    { id: 2, type: 'filter', pos_x: 0, pos_y: 100, config: { conditions: [{ type: 'keyword_excludes', config: { keywords: ['spam'] } }] } },
  ] }
  const projected = flowToLinearFlow(flow)
  assert.deepEqual(projected.filter_ids, [7])
  assert.equal(projected.conditions.length, 1)
  assert.equal(flow.nodes[1].config.conditions?.[0].type, 'keyword_excludes')
})
