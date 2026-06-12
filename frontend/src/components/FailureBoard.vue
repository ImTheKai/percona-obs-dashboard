<script setup lang="ts">
import { computed } from 'vue'
import PackageCard from './PackageCard.vue'
import GreenStrip from './GreenStrip.vue'
import type { Package } from '../types/api'

const props = defineProps<{ packages: Package[] }>()

const failingPackages = computed(() => props.packages.filter(p => p.rollup_state !== 'succeeded'))
const okCount = computed(() => props.packages.filter(p => p.rollup_state === 'succeeded').length)
</script>

<template>
  <div class="space-y-3">
    <PackageCard v-for="pkg in failingPackages" :key="`${pkg.project}/${pkg.name}`" :pkg="pkg" />
    <GreenStrip v-if="okCount > 0" :ok-count="okCount" />
    <div v-if="packages.length === 0" class="text-center text-text-muted py-8">
      No packages found
    </div>
  </div>
</template>
