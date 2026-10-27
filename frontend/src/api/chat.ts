import { getJSON, postJSON } from './client'
import type { ChatMessage } from './types'

export function fetchMessages(): Promise<ChatMessage[]> {
  return getJSON<ChatMessage[]>('/api/messages')
}

export function sendMessage(to: string, text: string): Promise<void> {
  return postJSON('/api/send', { to, text })
}
