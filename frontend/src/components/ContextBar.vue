<script setup lang="ts">
import ScopeChip from './ScopeChip.vue'

defineProps<{
  version: string
  updatedAt: string | null
  activeScopes: string[]
}>()

const emit = defineEmits<{
  'update:version': [version: string]
  'toggle-scope': [scope: string]
}>()

const VERSIONS = ['17', '18', '16']
const SCOPES = ['all', 'common', 'ppgcommon', 'version', 'container', 'release']

function formatTime(iso: string | null): string {
  if (!iso) return 'Never'
  return new Date(iso).toLocaleTimeString()
}
</script>

<template>
  <div class="bg-bg-card border-b border-border px-6 py-3 space-y-2">
    <div class="flex items-center gap-3 flex-wrap">
      <span class="px-2 py-0.5 rounded text-xs font-semibold bg-brand-purple text-white">PostgreSQL</span>

      <div class="flex rounded border border-border overflow-hidden">
        <button
          v-for="v in VERSIONS"
          :key="v"
          @click="emit('update:version', v)"
          :class="[
            'px-3 py-1 text-sm font-medium transition-colors',
            version === v
              ? 'bg-brand-purple text-white'
              : 'bg-bg-card text-text-secondary hover:bg-brand-purple-tint'
          ]"
        >{{ v }}</button>
      </div>

      <span class="text-xs text-text-muted font-mono">isv:percona</span>

      <span class="ml-auto text-xs text-text-muted">
        Updated: {{ formatTime(updatedAt) }}
      </span>
    </div>

    <div class="flex gap-2 flex-wrap">
      <ScopeChip
        v-for="scope in SCOPES"
        :key="scope"
        :scope="scope"
        :active="scope === 'all' ? activeScopes.length === 0 : activeScopes.includes(scope)"
        @toggle="scope === 'all' ? emit('toggle-scope', 'all') : emit('toggle-scope', scope)"
      />
    </div>
  </div>
</template>
