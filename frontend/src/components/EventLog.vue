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
  <div style="position: sticky; top: 16px; background: var(--bg-card); border: 1px solid var(--border); border-radius: 14px; display: flex; flex-direction: column; max-height: calc(100vh - 40px); overflow: hidden;">
    <!-- Header -->
    <div style="padding: 15px 16px 13px; border-bottom: 1px solid var(--border); display: flex; flex-direction: column; gap: 11px;">
      <div style="display: flex; align-items: center; gap: 9px;">
        <h2 style="margin: 0; font-size: 15px; font-weight: 700; color: var(--text-primary);">Build events</h2>
        <span style="font-size: 11.5px; color: var(--text-muted); font-family: var(--font-mono);">{{ events.length }} in window</span>
        <span style="margin-left: auto; display: inline-flex; align-items: center; gap: 6px; font-size: 11px; color: var(--text-muted);">
          <span style="width: 6px; height: 6px; border-radius: 99px; background: var(--ok);"></span>live
        </span>
      </div>
      <TimeWindowPicker
        :window-min="windowMin"
        :custom-from="customFrom"
        :custom-to="customTo"
        @update:window-min="emit('update:windowMin', $event)"
        @update:custom-from="emit('update:customFrom', $event)"
        @update:custom-to="emit('update:customTo', $event)"
      />
    </div>

    <!-- Scrollable event list -->
    <div style="overflow-y: auto; padding: 6px 4px 10px;">
      <div v-for="group in grouped" :key="group.bucket">
        <div style="padding: 11px 14px 5px; font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em;">{{ group.bucket }}</div>
        <EventRow v-for="event in group.events" :key="event.id" :event="event" />
      </div>
      <div v-if="grouped.length === 0" style="padding: 30px 16px; text-align: center; color: var(--text-muted); font-size: 13px;">
        No events in this time window
      </div>
    </div>
  </div>
</template>
