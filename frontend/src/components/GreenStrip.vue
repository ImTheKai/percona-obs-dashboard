<script setup lang="ts">
import type { Package } from '../types/api'
defineProps<{ packages: Package[] }>()
</script>

<template>
  <div style="background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; padding: 15px; display: flex; flex-direction: column; gap: 11px;">
    <div style="display: flex; align-items: center; gap: 9px;">
      <span style="width: 10px; height: 10px; border-radius: 3px; background: var(--ok);"></span>
      <span style="font-size: 13px; font-weight: 700; color: var(--text-primary);">All clear · {{ packages.length }} package{{ packages.length !== 1 ? 's' : '' }} fully built</span>
    </div>
    <div style="display: flex; gap: 7px; flex-wrap: wrap;">
      <span
        v-for="pkg in packages"
        :key="`${pkg.project}/${pkg.name}`"
        style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 7px; background: var(--ok-tint);"
      >
        <span style="width: 6px; height: 6px; border-radius: 99px; background: var(--ok);"></span>
        <code style="font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary);">{{ pkg.name }}</code>
      </span>
    </div>
  </div>
</template>
