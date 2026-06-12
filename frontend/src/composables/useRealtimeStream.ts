import { onMounted, onUnmounted, type Ref } from 'vue'
import type { Package, Event } from '../types/api'

export function useRealtimeStream(
  packages: Ref<Package[]>,
  events: Ref<Event[]>,
  refresh: () => void,
): void {
  let es: EventSource | null = null
  let wasError = false

  function connect(): void {
    es = new EventSource('/api/stream')

    es.onopen = (): void => {
      if (wasError) {
        wasError = false
        refresh()
      }
    }

    es.onmessage = (e: MessageEvent): void => {
      const msg = JSON.parse(e.data as string) as { type: string; data: unknown }

      if (msg.type === 'package_update') {
        const pkg = msg.data as Package
        const idx = packages.value.findIndex(
          (p) => p.project === pkg.project && p.name === pkg.name,
        )
        if (idx >= 0) {
          packages.value[idx] = pkg
        } else {
          packages.value.push(pkg)
        }
      } else if (msg.type === 'new_event') {
        events.value.unshift(msg.data as Event)
        if (events.value.length > 200) {
          events.value.length = 200
        }
      }
    }

    es.onerror = (): void => {
      wasError = true
      // EventSource reconnects automatically — no manual action needed.
    }
  }

  onMounted(connect)
  onUnmounted((): void => {
    es?.close()
  })
}
