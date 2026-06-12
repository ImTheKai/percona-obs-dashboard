<script setup lang="ts">
defineProps<{ theme: 'light' | 'dark' }>()
const emit = defineEmits<{ 'toggle-theme': [] }>()
</script>

<template>
  <header class="flex items-center justify-between px-6 py-3 bg-bg-card border-b border-border">
    <div class="flex items-center gap-3">
      <div class="w-8 h-8 rounded-full bg-brand-purple flex items-center justify-center text-white font-bold text-sm">P</div>
      <span class="font-semibold text-text-primary text-lg">PPG Build Board</span>
    </div>
    <button
      @click="emit('toggle-theme')"
      class="w-8 h-8 rounded flex items-center justify-center text-text-secondary hover:bg-brand-purple-tint"
      :title="theme === 'dark' ? 'Switch to light' : 'Switch to dark'"
    >
      {{ theme === 'dark' ? '☀' : '🌙' }}
    </button>
  </header>
</template>
