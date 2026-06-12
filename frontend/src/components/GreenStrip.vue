<script setup lang="ts">
defineProps<{ okCount: number }>()
</script>

<template>
  <div class="bg-ok-tint border border-ok rounded-lg px-4 py-3 flex items-center gap-2">
    <span class="text-ok font-medium">✓</span>
    <span class="text-ok font-medium text-sm">{{ okCount }} package{{ okCount !== 1 ? 's' : '' }} built successfully</span>
  </div>
</template>
