import type { ConditionConfig } from './index'

export interface ConditionResult {
  duration_us?: number
  path: string
  type: string
  status: string
  negate?: boolean
  field?: string
  evidence?: string
  reason?: string
  children?: ConditionResult[]
}

export interface FilterResult {
  id: number
  name: string
  revision: number
  hash: string
  result: ConditionResult
}

export interface NodeDecision {
  from_node_id?: number
  config_snapshot?: ConditionConfig[]
  node_id: number
  type: string
  status: string
  reason?: string
  filters?: FilterResult[]
  conditions?: ConditionResult
  duration_us: number
}

export interface Evaluation {
  truncated?: boolean
  version: number
  status: string
  reason?: string
  duration_us: number
  flows: { id: number; name: string; version: string; status: string; reason?: string; nodes: NodeDecision[] }[]
  targets: { flow_id: number; node_id: number; sink_id: number; task_id?: number; status: string; reason?: string }[]
}

export interface DecisionRecord {
  id: number
  message_id: number
  source_id: number
  message_revision: number
  trigger: string
  created_at: string
  evaluation: Evaluation
}

export interface FilterStats { passed: number; blocked: number; errors: number; references: number; since: string }
export interface FilterReference { kind: string; id: number; node_id?: number; name: string; enabled: boolean }
export interface FilterVersion { revision: number; name: string; description: string; conditions: ConditionConfig[]; created_at: string }

export interface PreviewMessage {
  source_id: number
  text: string
  message_type?: string
  sender_peer_type?: string
  sender_id?: number
  sender_name?: string
  links?: { url: string; title?: string }[]
  media?: Record<string, unknown>[]
  sent_at?: string
  received_at?: string
}

export interface FilterPreviewRequest {
  conditions: ConditionConfig[]
  compare_id?: number
  message?: PreviewMessage
  message_id?: number
  source_id?: number
  from?: string
  to?: string
  before_id?: number
  limit?: number
}

export interface FilterPreview {
  passed: number
  blocked: number
  changed: number
  config_hash: string
  previous_revision?: number
  has_more: boolean
  next_cursor?: number
  items: { message_id: number; message_text: string; result: ConditionResult; previous?: ConditionResult; changed: boolean }[]
}

export interface FlowPreview {
  matched: boolean
  evaluation: Evaluation
  targets: { flow_id: number; node_id: number; sink_id: number; text: string; status: string; media?: Record<string, unknown>[] }[]
}
