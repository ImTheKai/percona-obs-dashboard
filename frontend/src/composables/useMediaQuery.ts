import { ref, onMounted, onUnmounted, type Ref } from 'vue'

/**
 * Reactive wrapper around window.matchMedia.
 * Returns a ref that tracks whether `query` currently matches.
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const mql = window.matchMedia(query)
  const matches = ref(mql.matches)

  function update(e: MediaQueryListEvent) {
    matches.value = e.matches
  }

  onMounted(() => {
    matches.value = mql.matches
    mql.addEventListener('change', update)
  })
  onUnmounted(() => {
    mql.removeEventListener('change', update)
  })

  return matches
}
