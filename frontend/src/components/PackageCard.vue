<script setup lang="ts">
import { computed } from 'vue'
import type { Package, Target } from '../types/api'

const props = defineProps<{ pkg: Package }>()

const STATE_COLOR: Record<string, string> = {
  succeeded: 'var(--ok)',
  failed: 'var(--fail)',
  unresolvable: 'var(--warn)',
  broken: 'var(--broken)',
  blocked: 'var(--blocked)',
  building: 'var(--info)',
  scheduled: 'var(--info)',
}

const STATE_BG: Record<string, string> = {
  succeeded: 'var(--ok-tint)',
  failed: 'var(--fail-tint)',
  unresolvable: 'var(--warn-tint)',
  broken: 'var(--broken-tint)',
  blocked: 'var(--blocked-tint)',
  building: 'var(--info-tint)',
  scheduled: 'var(--info-tint)',
}

const failingTargets = computed(() =>
  props.pkg.targets.filter(t => t.state !== 'succeeded')
)

const visibleFailing = computed(() => failingTargets.value.slice(0, 3))
const hiddenFailingCount = computed(() => Math.max(0, failingTargets.value.length - 3))

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

// Group targets by repo for the grid
const repos = computed(() => {
  const map = new Map<string, Target[]>()
  for (const t of props.pkg.targets) {
    if (!map.has(t.repo)) map.set(t.repo, [])
    map.get(t.repo)!.push(t)
  }
  return map
})

const rollupColor = computed(() => STATE_COLOR[props.pkg.rollup_state] ?? 'var(--text-muted)')
const rollupBg = computed(() => STATE_BG[props.pkg.rollup_state] ?? 'var(--blocked-tint)')
</script>

<template>
  <div class="bg-bg-card rounded-lg border border-border p-4 space-y-3">
    <!-- Header: name + scope + rollup state -->
    <div class="flex items-center gap-2 justify-between">
      <div class="flex items-center gap-2 min-w-0">
        <span class="font-semibold text-text-primary truncate">{{ pkg.name }}</span>
        <span class="text-xs px-1.5 py-0.5 rounded text-text-muted bg-blocked-tint shrink-0">{{ pkg.scope }}</span>
      </div>
      <span
        class="text-xs font-medium px-2 py-0.5 rounded-full shrink-0"
        :style="{ color: rollupColor, backgroundColor: rollupBg }"
      >{{ pkg.rollup_state }}</span>
    </div>

    <!-- Trigger line -->
    <div v-if="pkg.trigger" class="text-xs text-text-secondary">
      ↻ <span class="text-text-primary">{{ pkg.trigger.what }}</span>
      <span class="text-text-muted ml-1">· {{ pkg.trigger.kind }} · {{ timeAgo(pkg.trigger.at) }}</span>
    </div>

    <!-- Target grid: repos × arches -->
    <div class="space-y-1">
      <div v-for="[repo, targets] in repos" :key="repo" class="flex items-center gap-1">
        <span class="text-xs text-text-muted w-36 truncate shrink-0">{{ repo }}</span>
        <div class="flex gap-1 flex-wrap">
          <span
            v-for="t in targets"
            :key="`${t.repo}-${t.arch}`"
            class="text-xs px-1.5 py-0.5 rounded font-mono"
            :style="{ color: STATE_COLOR[t.state] ?? 'var(--text-muted)', backgroundColor: STATE_BG[t.state] ?? 'var(--blocked-tint)' }"
            :title="`${t.arch}: ${t.state}`"
          >{{ t.arch }}</span>
        </div>
      </div>
    </div>

    <!-- Failing targets list: first 3 + N more -->
    <div v-if="failingTargets.length > 0" class="space-y-1">
      <div
        v-for="t in visibleFailing"
        :key="`fail-${t.repo}-${t.arch}`"
        class="text-xs flex items-center gap-1"
      >
        <span :style="{ color: STATE_COLOR[t.state] }">●</span>
        <span class="text-text-secondary">{{ t.repo }}/{{ t.arch }}</span>
        <span :style="{ color: STATE_COLOR[t.state] }">{{ t.state }}</span>
      </div>
      <div v-if="hiddenFailingCount > 0" class="text-xs text-text-muted">
        +{{ hiddenFailingCount }} more
      </div>
    </div>

    <!-- Ok count -->
    <div class="text-xs text-text-muted">
      {{ pkg.ok_targets }}/{{ pkg.total_targets }} targets ok
    </div>
  </div>
</template>
