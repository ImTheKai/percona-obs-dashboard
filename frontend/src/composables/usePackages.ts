import { ref, computed, toValue } from 'vue'
import type { MaybeRef } from 'vue'
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

// Known PPG version segments — used to exclude packages from other versions.
const KNOWN_VERSIONS = ['16', '17', '18']

// A package belongs to the selected version if its project path does not contain
// a segment that is a *different* version number. Common packages like
// isv:percona:ppg:common or isv:percona:common have no version segment and are
// always included.
function matchesVersion(pkg: Package, version: string): boolean {
  const segments = pkg.project.split(':')
  return !KNOWN_VERSIONS.filter(v => v !== version).some(v => segments.includes(v))
}

export function usePackages(product: MaybeRef<string>, version: MaybeRef<string>) {
  const data = ref<Package[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    const p = toValue(product)
    const v = toValue(version)
    loading.value = true
    error.value = null
    try {
      const res = await fetch(`/api/products/${p}/${v}/packages`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      data.value = await res.json()
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  const sorted = computed(() => {
    const ver = toValue(version)
    return [...data.value]
      .filter(pkg => matchesVersion(pkg, ver))
      .sort((a, b) => (SEVERITY[b.rollup_state] ?? 0) - (SEVERITY[a.rollup_state] ?? 0))
  })

  function filterByScope(scopes: string[]) {
    if (scopes.length === 0) return sorted.value
    return sorted.value.filter(p => scopes.includes(p.scope))
  }

  return { data: sorted, loading, error, refresh, filterByScope }
}
