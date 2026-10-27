import { getJSON } from './client'
import type { Me, Peer } from './types'

export function fetchMe(): Promise<Me> {
  return getJSON<Me>('/api/me')
}

export function fetchPeers(): Promise<Peer[]> {
  return getJSON<Peer[]>('/api/peers')
}
