<script setup lang="ts">
import { computed } from 'vue'
import TimeWindowPicker from './TimeWindowPicker.vue'
import EventRow from './EventRow.vue'
import type { Event } from '../types/api'

const props = defineProps<{
  events: Event[]
  windowMin: number
  customFrom: string | null
  customTo: string | null
}>()

const emit = defineEmits<{
  'update:windowMin': [min: number]
  'update:customFrom': [date: string]
  'update:customTo': [date: string]
}>()

type Bucket = 'Today' | 'Yesterday' | 'Earlier'

function getBucket(iso: string): Bucket {
  const d = new Date(iso)
  const now = new Date()
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const yesterdayStart = new Date(todayStart.getTime() - 86400000)
  if (d >= todayStart) return 'Today'
  if (d >= yesterdayStart) return 'Yesterday'
  return 'Earlier'
}

const grouped = computed(() => {
  const groups: { bucket: Bucket; events: Event[] }[] = [
    { bucket: 'Today', events: [] },
    { bucket: 'Yesterday', events: [] },
    { bucket: 'Earlier', events: [] },
  ]
  for (const e of props.events) {
    const b = getBucket(e.at)
    groups.find(g => g.bucket === b)!.events.push(e)
  }
  return groups.filter(g => g.events.length > 0)
})
</script>

<template>
  <div class="flex flex-col h-full bg-bg-card rounded-lg border border-border p-4 gap-3">
    <div class="flex items-center justify-between">
      <span class="font-semibold text-text-primary">Event Log</span>
    </div>
    <TimeWindowPicker
      :window-min="windowMin"
      :custom-from="customFrom"
      :custom-to="customTo"
      @update:window-min="emit('update:windowMin', $event)"
      @update:custom-from="emit('update:customFrom', $event)"
      @update:custom-to="emit('update:customTo', $event)"
    />
    <div class="overflow-y-auto flex-1">
      <div v-for="group in grouped" :key="group.bucket" class="mb-3">
        <div class="text-xs font-semibold text-text-muted uppercase tracking-wide mb-1 pb-1 border-b border-border">
          {{ group.bucket }}
        </div>
        <EventRow v-for="event in group.events" :key="event.id" :event="event" />
      </div>
      <div v-if="grouped.length === 0" class="text-center text-text-muted py-8 text-sm">
        No events in this time window
      </div>
    </div>
  </div>
</template>
