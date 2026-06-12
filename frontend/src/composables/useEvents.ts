import { ref } from 'vue'
import type { Event } from '../types/api'

export function useEvents(product: string, version: string) {
  const data = ref<Event[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh(opts: { window?: number; from?: string; to?: string } = {}) {
    loading.value = true
    error.value = null
    try {
      let qs = ''
      if (opts.from && opts.to) {
        qs = `?from=${encodeURIComponent(opts.from)}&to=${encodeURIComponent(opts.to)}`
      } else {
        qs = `?window=${opts.window ?? 1440}`
      }
      const res = await fetch(`/api/products/${product}/${version}/events${qs}`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      data.value = await res.json()
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  return { data, loading, error, refresh }
}
