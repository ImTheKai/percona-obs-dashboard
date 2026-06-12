<script setup lang="ts">
import type { Event, EventType } from '../types/api'

defineProps<{ event: Event }>()

const GLYPH: Record<EventType, string> = {
  succeeded: '✓', failed: '✗', broken: '✗', unresolvable: '⚠',
  blocked: '⊘', published: '↑', triggered: '↻', started: '▶',
}

const GLYPH_COLOR: Record<EventType, string> = {
  succeeded: 'var(--ok)', failed: 'var(--fail)', broken: 'var(--broken)',
  unresolvable: 'var(--warn)', blocked: 'var(--blocked)',
  published: 'var(--info)', triggered: 'var(--brand-purple)', started: 'var(--info)',
}

function timeStr(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="flex gap-2 py-2 border-b border-border last:border-0">
    <div class="w-5 h-5 rounded-full flex items-center justify-center text-xs shrink-0 mt-0.5"
         :style="{ color: GLYPH_COLOR[event.type], backgroundColor: GLYPH_COLOR[event.type] + '22' }">
      {{ GLYPH[event.type] }}
    </div>
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-1.5 flex-wrap">
        <span class="text-xs text-text-muted font-mono">{{ event.scope }}</span>
        <span class="text-sm font-medium text-text-primary truncate">{{ event.what }}</span>
      </div>
      <div class="text-xs text-text-secondary mt-0.5">{{ event.why }}</div>
      <div class="flex items-center gap-2 mt-1">
        <span v-if="event.repo" class="text-xs text-text-muted">{{ event.repo }}/{{ event.arch }}</span>
        <a :href="event.url" target="_blank" class="text-xs text-brand-purple hover:underline ml-auto">↗</a>
        <span class="text-xs text-text-muted">{{ timeStr(event.at) }}</span>
      </div>
    </div>
  </div>
</template>
