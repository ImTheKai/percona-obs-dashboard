import { ref, computed } from 'vue'
import type { Package } from '../types/api'

const SEVERITY: Record<string, number> = {
  broken: 5,
  unresolvable: 4,
  failed: 3,
  blocked: 2,
  building: 1,
  scheduled: 1,
  succeeded: 0,
}

export function usePackages(product: string, version: string) {
  const data = ref<Package[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const res = await fetch(`/api/products/${product}/${version}/packages`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      data.value = await res.json()
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  const sorted = computed(() =>
    [...data.value].sort((a, b) =>
      (SEVERITY[b.rollup_state] ?? 0) - (SEVERITY[a.rollup_state] ?? 0)
    )
  )

  function filterByScope(scopes: string[]) {
    if (scopes.length === 0) return sorted.value
    return sorted.value.filter(p => scopes.includes(p.scope))
  }

  return { data: sorted, loading, error, refresh, filterByScope }
}
