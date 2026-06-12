<script setup lang="ts">
const props = defineProps<{ scope: string; active: boolean }>()
defineEmits<{ toggle: [scope: string] }>()
</script>

<template>
  <button
    @click="$emit('toggle', props.scope)"
    :class="[
      'px-3 py-1 rounded-full text-xs font-medium border transition-colors',
      active
        ? 'bg-brand-purple text-white border-brand-purple'
        : 'bg-bg-card text-text-secondary border-border hover:border-brand-purple hover:text-brand-purple'
    ]"
  >
    {{ scope }}
  </button>
</template>
