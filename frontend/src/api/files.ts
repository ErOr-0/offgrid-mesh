import { getJSON, nodeBase } from './client'
import type { FileInfo } from './types'

export function fetchFiles(): Promise<FileInfo[]> {
  return getJSON<FileInfo[]>('/api/files')
}

export function fileDownloadURL(id: string): string {
  return `${nodeBase}/api/files/${id}/download`
}

export function sendFileWithProgress(
  to: string,
  file: File,
  onProgress: (pct: number) => void,
): Promise<void> {
  return file.arrayBuffer().then(
    (buf) =>
      new Promise<void>((resolve, reject) => {
        const bytes = new Uint8Array(buf)
        let binary = ''
        bytes.forEach((b) => (binary += String.fromCharCode(b)))
        const xhr = new XMLHttpRequest()
        xhr.open('POST', `${nodeBase}/api/files/send`)
        xhr.setRequestHeader('Content-Type', 'application/json')
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
        }
        xhr.onload = () =>
          xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(xhr.responseText || 'send failed'))
        xhr.onerror = () => reject(new Error('send failed'))
        xhr.send(JSON.stringify({ to, name: file.name, data: btoa(binary) }))
      }),
  )
}
