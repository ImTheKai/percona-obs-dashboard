<script setup lang="ts">
import type { Context } from '../types/api'

defineProps<{
  version: string
  updatedAt: string | null
  activeScopes: string[]
  contexts: Context[]
  selectedContext: Context
  availableVersions: string[]
}>()

const emit = defineEmits<{
  'update:version': [version: string]
  'toggle-scope': [scope: string]
  'update:context': [ctx: Context]
}>()

const SCOPES = [
  { id: 'all', label: 'All' },
  { id: 'common', label: 'Common' },
  { id: 'ppgcommon', label: 'PPG Common' },
  { id: 'version', label: 'Version' },
  { id: 'container', label: 'Container' },
  { id: 'release', label: 'Release' },
]

function formatTime(iso: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return `${time} · ${isToday ? 'today' : d.toLocaleDateString()}`
}

function tabStyle(v: string, selected: string): string {
  const active = v === selected
  return active
    ? 'background: var(--bg-card); color: var(--text-primary); font-weight: 700; padding: 4px 12px; border-radius: 7px; border: none; font-size: 13px; cursor: pointer; font-family: inherit;'
    : 'background: transparent; color: var(--text-muted); font-weight: 500; padding: 4px 12px; border-radius: 7px; border: none; font-size: 13px; cursor: pointer; font-family: inherit;'
}

function scopeStyle(id: string, active: boolean): string {
  return active
    ? 'background: var(--brand-purple); color: #fff; padding: 4px 11px; border-radius: 8px; border: none; font-size: 11.5px; font-weight: 600; cursor: pointer; font-family: inherit;'
    : 'background: transparent; color: var(--text-secondary); padding: 4px 11px; border-radius: 8px; border: 1px solid var(--border); font-size: 11.5px; font-weight: 500; cursor: pointer; font-family: inherit;'
}
</script>

<template>
  <div style="background: var(--bg-card); border: 1px solid var(--border); border-radius: 14px; padding: 14px 18px; display: flex; flex-direction: column; gap: 13px;">
    <!-- Top row: tech badge + context selector + version tabs + updated -->
    <div style="display: flex; align-items: center; gap: 16px; flex-wrap: wrap;">
      <span style="display: inline-flex; align-items: center; gap: 7px; padding: 5px 12px; border-radius: 8px; background: var(--tint-postgres); color: var(--tech-postgres); font-size: 12px; font-weight: 700; border: 1px solid rgba(0,94,214,0.15);">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" style="flex-shrink:0;">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z" fill="currentColor" opacity="0.15"/>
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" fill="currentColor"/>
          <text x="7" y="16" font-size="10" font-weight="800" fill="currentColor" font-family="monospace">pg</text>
        </svg>
        PostgreSQL
      </span>

      <!-- Context selector: dropdown when multiple contexts exist, plain badge otherwise -->
      <select
        v-if="contexts.length > 1"
        :value="selectedContext.apiBase"
        @change="e => { const apiBase = (e.target as HTMLSelectElement).value; const ctx = contexts.find(c => c.apiBase === apiBase); if (ctx) emit('update:context', ctx) }"
        style="font-family: var(--font-mono); font-size: 12.5px; color: var(--text-secondary); background: var(--bg-muted); padding: 5px 10px; border-radius: 7px; border: 1px solid var(--border); cursor: pointer;"
      >
        <option v-for="ctx in contexts" :key="ctx.apiBase" :value="ctx.apiBase">{{ ctx.prefix }}</option>
      </select>
      <code
        v-else
        style="font-family: var(--font-mono); font-size: 12.5px; color: var(--text-secondary); background: var(--bg-muted); padding: 5px 10px; border-radius: 7px;"
      >{{ selectedContext.prefix }}</code>

      <!-- Version tabs: hidden when no versioned packages exist in the context -->
      <div v-if="availableVersions.length > 0" style="display: flex; align-items: center; gap: 6px;">
        <span style="font-size: 11px; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; margin-right: 2px;">Version</span>
        <div style="display: flex; gap: 3px; background: var(--bg-muted); padding: 3px; border-radius: 9px;">
          <button
            v-for="v in availableVersions"
            :key="v"
            @click="emit('update:version', v)"
            :style="tabStyle(v, version)"
          >{{ v }}</button>
        </div>
      </div>

      <div style="margin-left: auto; display: flex; align-items: center; gap: 16px; font-size: 12px; color: var(--text-muted);">
        <span>Updated <strong style="color: var(--text-secondary); font-weight: 600;">{{ formatTime(updatedAt) }}</strong></span>
        <span style="display: inline-flex; align-items: center; gap: 6px;">
          <span style="width: 7px; height: 7px; border-radius: 99px; background: var(--ok);"></span>Auto-refresh 5 min
        </span>
      </div>
    </div>

    <!-- Scope chips -->
    <div style="display: flex; align-items: center; gap: 9px; flex-wrap: wrap; border-top: 1px solid var(--border); padding-top: 12px;">
      <span style="font-size: 11px; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; margin-right: 2px;">Scope</span>
      <button
        v-for="s in SCOPES"
        :key="s.id"
        @click="s.id === 'all' ? emit('toggle-scope', 'all') : emit('toggle-scope', s.id)"
        :style="scopeStyle(s.id, s.id === 'all' ? activeScopes.length === 0 : activeScopes.includes(s.id))"
      >{{ s.label }}</button>
    </div>
  </div>
</template>
