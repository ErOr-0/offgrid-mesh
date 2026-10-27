import { getJSON } from './client'
import type { RoutingEntry } from './types'

export function fetchRouting(): Promise<RoutingEntry[]> {
  return getJSON<RoutingEntry[]>('/api/routing')
}
