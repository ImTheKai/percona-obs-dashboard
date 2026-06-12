<script setup lang="ts">
defineProps<{ theme: 'light' | 'dark' }>()
const emit = defineEmits<{ 'toggle-theme': [] }>()
</script>

<template>
  <header style="display: flex; align-items: center; justify-content: space-between; gap: 20px;">
    <div style="display: flex; align-items: center; gap: 12px;">
      <div style="width: 34px; height: 34px; border-radius: 9px; background: var(--brand-purple); display: flex; align-items: center; justify-content: center; color: #fff; font-weight: 800; font-size: 17px; font-family: 'Roboto Condensed', 'Roboto', sans-serif;">P</div>
      <div style="display: flex; flex-direction: column; gap: 1px;">
        <h1 style="margin: 0; font-size: 21px; font-weight: 700; letter-spacing: -0.01em; color: var(--text-primary);">PPG Build Board</h1>
        <span style="font-size: 12.5px; color: var(--text-muted);">Failure-first build monitor across every subproject of a product</span>
      </div>
    </div>
    <button
      @click="emit('toggle-theme')"
      style="flex-shrink: 0; display: inline-flex; align-items: center; gap: 8px; padding: 8px 14px; border-radius: 10px; border: 1px solid var(--border); background: var(--bg-card); color: var(--text-secondary); font-family: inherit; font-size: 13px; font-weight: 600; cursor: pointer;"
    >
      <span style="width: 8px; height: 8px; border-radius: 99px; background: var(--brand-purple);"></span>
      {{ theme === 'dark' ? 'Dark' : 'Light' }} mode
    </button>
  </header>
</template>
