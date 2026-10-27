import { getJSON, postJSON } from './client'
import type { BundleInfo } from './types'

export function fetchBundles(): Promise<BundleInfo[]> {
  return getJSON<BundleInfo[]>('/api/bundles')
}

export function queueBundle(to: string, region: string, text: string): Promise<void> {
  return postJSON('/api/bundles', { to, region, text, copies: 8, ttl_hours: 48 })
}

// Queue a bundle addressed to a destination IP:port (e.g. a node in Malaysia).
// Carriers deliver it when one of them can finally reach that IP.
export function queueBundleDestIP(destIP: string, region: string, text: string): Promise<void> {
  return postJSON('/api/bundles', { dest_ip: destIP, region, text, copies: 8, ttl_hours: 48 })
}

// LooksLikeIPPort reports whether a destination string is an IP:port address
// rather than a device ID (device IDs are hex, IPs contain '.' or ':').
export function looksLikeIPPort(dest: string): boolean {
  return dest.includes('.') || dest.includes(':')
}
