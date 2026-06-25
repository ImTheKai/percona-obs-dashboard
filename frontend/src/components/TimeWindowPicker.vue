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
  <div class="flex flex-col gap-[9px]">
    <div class="flex items-center gap-[7px] flex-wrap">
      <span class="text-[10.5px] text-text-muted font-semibold uppercase tracking-[0.05em]">Window</span>
      <div class="flex gap-[3px] bg-bg-muted p-[3px] rounded-[9px] flex-wrap">
        <button
          v-for="p in PRESETS"
          :key="p.label"
          @click="emit('update:windowMin', p.min)"
          :class="windowMin === p.min
            ? 'bg-bg-card text-text-primary font-bold py-[4px] px-[10px] rounded-[7px] border-none text-[12px] cursor-pointer'
            : 'bg-transparent text-text-muted font-medium py-[4px] px-[10px] rounded-[7px] border-none text-[12px] cursor-pointer'"
        >{{ p.label }}</button>
      </div>
    </div>
    <div v-if="isCustom" class="flex items-center gap-[8px] flex-wrap bg-bg-card-2 border border-border rounded-[10px] py-[9px] px-[11px]">
      <div class="flex flex-col gap-[3px]">
        <label class="text-[9.5px] text-text-muted font-bold uppercase tracking-[0.05em]">From</label>
        <input
          type="date"
          :value="customFrom ?? ''"
          @change="emit('update:customFrom', ($event.target as HTMLInputElement).value)"
          class="font-mono text-[12px] text-text-primary bg-bg-card border border-border-strong rounded-[7px] py-[5px] px-[8px]"
        />
      </div>
      <span class="text-[13px] text-text-muted self-end pb-[6px]">→</span>
      <div class="flex flex-col gap-[3px]">
        <label class="text-[9.5px] text-text-muted font-bold uppercase tracking-[0.05em]">To</label>
        <input
          type="date"
          :value="customTo ?? ''"
          @change="emit('update:customTo', ($event.target as HTMLInputElement).value)"
          class="font-mono text-[12px] text-text-primary bg-bg-card border border-border-strong rounded-[7px] py-[5px] px-[8px]"
        />
      </div>
    </div>
  </div>
</template>
