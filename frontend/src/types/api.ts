export interface Trigger {
  what: string
  kind: string
  at: string // ISO 8601
}

export interface Target {
  repo: string
  arch: string
  state: string // 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'scheduled' | 'building'
}

export interface Package {
  project: string
  name: string
  scope: string // 'common' | 'ppgcommon' | 'version' | 'container' | 'release'
  rollup_state: string // 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'scheduled' | 'building'
  ok_targets: number
  total_targets: number
  trigger?: Trigger // optional
  targets: Target[]
  updated_at: string // ISO 8601
}

export interface Event {
  id: string
  type: string // 'triggered' | 'started' | 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'published'
  scope: string
  project: string
  package: string
  repo?: string // optional
  arch?: string // optional
  what: string
  why: string
  url: string
  at: string // ISO 8601
}
