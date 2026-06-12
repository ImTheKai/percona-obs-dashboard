<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  windowMin: number
  customFrom: string | null
  customTo: string | null
}>()

const emit = defineEmits<{
  'update:windowMin': [min: number]
  'update:customFrom': [date: string]
  'update:customTo': [date: string]
}>()

const PRESETS = [
  { label: '1h', min: 60 },
  { label: '6h', min: 360 },
  { label: '24h', min: 1440 },
  { label: '3d', min: 4320 },
  { label: '7d', min: 10080 },
  { label: 'Custom', min: -1 },
]

const isCustom = computed(() => props.windowMin === -1)
</script>

<template>
  <div style="display: flex; flex-direction: column; gap: 9px;">
    <div style="display: flex; align-items: center; gap: 7px; flex-wrap: wrap;">
      <span style="font-size: 10.5px; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em;">Window</span>
      <div style="display: flex; gap: 3px; background: var(--bg-muted); padding: 3px; border-radius: 9px; flex-wrap: wrap;">
        <button
          v-for="p in PRESETS"
          :key="p.label"
          @click="emit('update:windowMin', p.min)"
          :style="windowMin === p.min
            ? 'background: var(--bg-card); color: var(--text-primary); font-weight: 700; padding: 4px 10px; border-radius: 7px; border: none; font-size: 12px; cursor: pointer; font-family: inherit;'
            : 'background: transparent; color: var(--text-muted); font-weight: 500; padding: 4px 10px; border-radius: 7px; border: none; font-size: 12px; cursor: pointer; font-family: inherit;'"
        >{{ p.label }}</button>
      </div>
    </div>
    <div v-if="isCustom" style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap; background: var(--bg-card-2); border: 1px solid var(--border); border-radius: 10px; padding: 9px 11px;">
      <div style="display: flex; flex-direction: column; gap: 3px;">
        <label style="font-size: 9.5px; color: var(--text-muted); font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em;">From</label>
        <input
          type="date"
          :value="customFrom ?? ''"
          @change="emit('update:customFrom', ($event.target as HTMLInputElement).value)"
          style="font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-strong); border-radius: 7px; padding: 5px 8px;"
        />
      </div>
      <span style="font-size: 13px; color: var(--text-muted); align-self: flex-end; padding-bottom: 6px;">→</span>
      <div style="display: flex; flex-direction: column; gap: 3px;">
        <label style="font-size: 9.5px; color: var(--text-muted); font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em;">To</label>
        <input
          type="date"
          :value="customTo ?? ''"
          @change="emit('update:customTo', ($event.target as HTMLInputElement).value)"
          style="font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); background: var(--bg-card); border: 1px solid var(--border-strong); border-radius: 7px; padding: 5px 8px;"
        />
      </div>
    </div>
  </div>
</template>
