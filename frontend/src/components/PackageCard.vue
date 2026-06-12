<script setup lang="ts">
import { computed } from 'vue'
import type { Package } from '../types/api'

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

const STATE_LABEL: Record<string, string> = {
  succeeded: 'Succeeded', failed: 'Failed', unresolvable: 'Unresolvable',
  broken: 'Broken', blocked: 'Blocked', building: 'Building', scheduled: 'Scheduled',
}

const SCOPE_LABEL: Record<string, string> = {
  common: 'Common', ppgcommon: 'PPG Common', version: 'Version',
  container: 'Container', release: 'Release',
}

const failingTargets = computed(() =>
  props.pkg.targets.filter(t => t.state !== 'succeeded')
)
const visibleFailing = computed(() => failingTargets.value.slice(0, 3))
const hiddenFailingCount = computed(() => Math.max(0, failingTargets.value.length - 3))

const rollupColor = computed(() => STATE_COLOR[props.pkg.rollup_state] ?? 'var(--text-muted)')
const rollupBg = computed(() => STATE_BG[props.pkg.rollup_state] ?? 'var(--blocked-tint)')
const obsUrl = computed(() => `https://build.opensuse.org/package/show/${props.pkg.project}/${props.pkg.name}`)

function logUrl(repo: string, arch: string): string {
  return `https://build.opensuse.org/package/live_build_log/${props.pkg.project}/${props.pkg.name}/${repo}/${arch}`
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}
</script>

<template>
  <div :style="{
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderLeft: `4px solid ${rollupColor}`,
    borderRadius: '12px',
    padding: '15px',
    display: 'flex',
    flexDirection: 'column',
    gap: '11px',
  }">
    <!-- Row 1: state pill + name + OBS link -->
    <div style="display: flex; align-items: center; gap: 9px;">
      <span :style="{
        fontSize: '10.5px', fontWeight: '700', textTransform: 'uppercase',
        letterSpacing: '0.04em', padding: '3px 9px', borderRadius: '6px',
        color: rollupColor, background: rollupBg,
      }">{{ STATE_LABEL[pkg.rollup_state] ?? pkg.rollup_state }}</span>
      <code style="font-family: var(--font-mono); font-size: 13.5px; font-weight: 600; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ pkg.name }}</code>
      <a :href="obsUrl" target="_blank" rel="noopener" style="margin-left: auto; font-size: 11.5px; font-weight: 700; color: var(--brand-purple); text-decoration: none; white-space: nowrap; flex-shrink: 0;">OBS ↗</a>
    </div>

    <!-- Row 2: scope tag + project path -->
    <div style="display: flex; align-items: center; gap: 7px;">
      <span style="font-size: 9.5px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em; padding: 2px 7px; border-radius: 5px; background: var(--blocked-tint); color: var(--blocked);">{{ SCOPE_LABEL[pkg.scope] ?? pkg.scope }}</span>
      <code style="font-family: var(--font-mono); font-size: 10.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ pkg.project }}</code>
    </div>

    <!-- Row 3: trigger box -->
    <div v-if="pkg.trigger" style="display: flex; align-items: flex-start; gap: 8px; background: var(--bg-card-2); border: 1px solid var(--border); border-radius: 9px; padding: 9px 11px;">
      <span style="color: var(--warn); font-weight: 700; font-size: 13px; line-height: 1.3; flex-shrink: 0;">↻</span>
      <div style="display: flex; flex-direction: column; gap: 1px; min-width: 0;">
        <span style="font-size: 12px; color: var(--text-secondary);">Triggered by <strong style="color: var(--text-primary); font-weight: 700;">{{ pkg.trigger.what }}</strong></span>
        <span style="font-size: 10.5px; color: var(--text-muted);">{{ pkg.trigger.kind }} · {{ timeAgo(pkg.trigger.at) }}</span>
      </div>
    </div>

    <!-- Row 4: failing targets -->
    <div v-if="failingTargets.length > 0" style="display: flex; flex-direction: column; gap: 6px;">
      <span style="font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em;">
        {{ failingTargets.length }} failing target{{ failingTargets.length !== 1 ? 's' : '' }}
      </span>
      <div style="display: flex; flex-direction: column; gap: 5px;">
        <a
          v-for="t in visibleFailing"
          :key="`${t.repo}-${t.arch}`"
          :href="logUrl(t.repo, t.arch)"
          target="_blank"
          rel="noopener"
          :style="{
            display: 'flex', alignItems: 'center', gap: '9px',
            textDecoration: 'none', padding: '5px 9px', borderRadius: '7px',
            background: STATE_BG[t.state] ?? 'var(--blocked-tint)',
          }"
        >
          <span :style="{ width: '8px', height: '8px', borderRadius: '2px', background: STATE_COLOR[t.state] ?? 'var(--blocked)', flexShrink: '0' }"></span>
          <code style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-primary); flex-shrink: 0;">{{ t.repo }}/{{ t.arch }}</code>
          <span :style="{ fontSize: '11px', color: STATE_COLOR[t.state] ?? 'var(--text-secondary)', marginLeft: 'auto', fontWeight: '600', flexShrink: '0' }">{{ t.state }}</span>
          <span style="font-size: 10.5px; color: var(--brand-purple); font-weight: 700; flex-shrink: 0;">log ↗</span>
        </a>
        <span v-if="hiddenFailingCount > 0" style="font-size: 11px; color: var(--text-muted); padding: 2px 9px;">+{{ hiddenFailingCount }} more</span>
      </div>
    </div>

    <!-- Row 5: ok targets count -->
    <div style="font-size: 11px; color: var(--text-muted);">{{ pkg.ok_targets }}/{{ pkg.total_targets }} targets ok</div>
  </div>
</template>
