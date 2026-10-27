import { getJSON, nodeBase, postJSON } from './client'

export function allowDevice(id: string): Promise<void> {
  return postJSON('/api/allow', { id })
}

export function pairQRURL(): string {
  return `${nodeBase}/api/pair-qr`
}

export async function fetchPairCode(): Promise<string> {
  const data = await getJSON<{ code: string }>('/api/pair-code')
  return data.code
}

export function pairWithCode(code: string): Promise<void> {
  return postJSON('/api/pair', { code })
}
