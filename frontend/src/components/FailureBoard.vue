<script setup lang="ts">
import { computed } from 'vue'
import PackageCard from './PackageCard.vue'
import GreenStrip from './GreenStrip.vue'
import type { Package } from '../types/api'

const props = defineProps<{ packages: Package[]; spotlightStates: string[] }>()

const failingPackages = computed(() => props.packages.filter(p => p.rollup_state !== 'succeeded' && p.rollup_state !== 'published'))
const okPackages = computed(() => props.packages.filter(p => p.rollup_state === 'succeeded' || p.rollup_state === 'published'))
const attentionCount = computed(() => failingPackages.value.length)
</script>

<template>
  <div class="flex flex-col gap-[14px] min-w-0">
    <!-- Section header -->
    <div class="flex items-center gap-[10px]">
      <h2 class="m-0 text-[15px] font-bold text-text-primary">Active packages</h2>
      <span class="text-[12.5px] text-text-muted">{{ attentionCount }} package{{ attentionCount !== 1 ? 's' : '' }} · sorted by severity</span>
    </div>

    <!-- 2-column failure grid -->
    <div v-if="failingPackages.length > 0" class="grid grid-cols-1 sm:grid-cols-[repeat(2,minmax(0,1fr))] gap-[14px]">
      <PackageCard
        v-for="pkg in failingPackages"
        :key="`${pkg.project}/${pkg.name}`"
        :pkg="pkg"
        :spotlight-states="spotlightStates"
      />
    </div>

    <!-- All green state -->
    <div v-if="failingPackages.length === 0 && packages.length > 0" class="bg-ok-tint border border-ok rounded-[12px] p-7 flex flex-col items-center gap-2 text-center">
      <span class="text-[26px] text-ok font-extrabold">✓</span>
      <span class="text-[15px] font-bold text-text-primary">All packages green</span>
    </div>

    <!-- Empty state -->
    <div v-if="packages.length === 0" class="text-center text-text-muted py-8 text-sm">
      No packages found
    </div>

    <!-- Green strip -->
    <GreenStrip
      v-if="okPackages.length > 0"
      :packages="okPackages"
      :style="spotlightStates.length > 0 ? 'opacity: 0.2; transition: opacity 0.2s' : 'transition: opacity 0.2s'"
    />
  </div>
</template>
