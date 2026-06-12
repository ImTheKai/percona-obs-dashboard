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
  <div class="space-y-2">
    <div class="flex gap-1 flex-wrap">
      <button
        v-for="p in PRESETS"
        :key="p.label"
        @click="emit('update:windowMin', p.min)"
        :class="[
          'px-2.5 py-1 rounded text-xs font-medium transition-colors',
          windowMin === p.min
            ? 'bg-brand-purple text-white'
            : 'bg-bg-card border border-border text-text-secondary hover:border-brand-purple'
        ]"
      >{{ p.label }}</button>
    </div>
    <div v-if="isCustom" class="flex gap-2 items-center">
      <input
        type="date"
        :value="customFrom ?? ''"
        @change="emit('update:customFrom', ($event.target as HTMLInputElement).value)"
        class="text-xs border border-border rounded px-2 py-1 bg-bg-card text-text-primary"
      />
      <span class="text-text-muted text-xs">→</span>
      <input
        type="date"
        :value="customTo ?? ''"
        @change="emit('update:customTo', ($event.target as HTMLInputElement).value)"
        class="text-xs border border-border rounded px-2 py-1 bg-bg-card text-text-primary"
      />
    </div>
  </div>
</template>
