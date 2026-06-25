<script setup lang="ts">
import type { Context } from '../types/api'

defineProps<{
  version: string
  availableVersions: string[]
  activeTab: 'packages' | 'containers'
  contexts: Context[]
  selectedContext: Context
}>()

const emit = defineEmits<{
  'update:version': [v: string]
  'update:tab': [tab: 'packages' | 'containers']
  'update:context': [ctx: Context]
}>()
</script>

<template>
  <div class="bg-bg-card border border-border rounded-[14px] px-[18px] py-[14px] mx-4 my-3 flex-shrink-0">
    <div class="flex items-center gap-4 flex-wrap">
      <!-- PostgreSQL badge -->
      <span class="inline-flex items-center gap-[7px] px-3 py-[5px] rounded-[8px] bg-tint-postgres text-tech-postgres text-[12px] font-bold border border-[rgba(0,94,214,0.15)]">PostgreSQL</span>

      <!-- Context: plain badge when only one context, dropdown when multiple -->
      <code v-if="contexts.length <= 1" class="font-mono text-[12.5px] text-text-secondary bg-bg-muted px-[10px] py-[5px] rounded-[7px]">
        {{ selectedContext.prefix }}:{{ version }}
      </code>
      <select
        v-else
        class="font-mono text-[12.5px] text-text-secondary bg-bg-muted px-[10px] py-[5px] rounded-[7px] border-none cursor-pointer [appearance:auto]"
        :value="selectedContext.apiBase"
        @change="emit('update:context', contexts.find(c => c.apiBase === ($event.target as HTMLSelectElement).value)!)"
      >
        <option
          v-for="ctx in contexts"
          :key="ctx.apiBase"
          :value="ctx.apiBase"
        >{{ ctx.label }}</option>
      </select>

      <!-- Version segment control -->
      <div v-if="availableVersions.length > 0" class="flex items-center gap-[6px]">
        <span class="text-[11px] text-text-muted font-semibold uppercase [letter-spacing:0.06em] mr-[2px]">Version</span>
        <div class="flex gap-[3px] bg-bg-muted p-[3px] rounded-[9px] border border-border">
          <button
            v-for="v in availableVersions"
            :key="v"
            class="bg-transparent text-text-muted font-medium px-3 py-1 rounded-[7px] border border-transparent text-[13px] cursor-pointer [font-family:inherit]"
            :class="{ 'bg-bg-card text-text-primary font-bold border-border-strong shadow-[0_1px_2px_rgba(0,0,0,0.12)]': v === version }"
            @click="emit('update:version', v)"
          >{{ v }}</button>
        </div>
      </div>

      <!-- Tab switcher -->
      <div class="flex items-center gap-[6px] ml-auto">
        <div class="flex gap-[3px] bg-bg-muted p-[3px] rounded-[9px] border border-border">
          <button
            class="bg-transparent text-text-muted font-medium px-3 py-1 rounded-[7px] border border-transparent text-[13px] cursor-pointer [font-family:inherit]"
            :class="{ 'bg-bg-card text-text-primary font-bold border-border-strong shadow-[0_1px_2px_rgba(0,0,0,0.12)]': activeTab === 'packages' }"
            @click="emit('update:tab', 'packages')"
          >Packages</button>
          <button
            class="bg-transparent text-text-muted font-medium px-3 py-1 rounded-[7px] border border-transparent text-[13px] cursor-pointer [font-family:inherit]"
            :class="{ 'bg-bg-card text-text-primary font-bold border-border-strong shadow-[0_1px_2px_rgba(0,0,0,0.12)]': activeTab === 'containers' }"
            @click="emit('update:tab', 'containers')"
          >Container Images</button>
        </div>
      </div>
    </div>
  </div>
</template>
