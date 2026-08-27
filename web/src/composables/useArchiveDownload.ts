import { getToken } from '@/api/client'

interface SaveFileHandle {
  createWritable(): Promise<WritableStream>
}

interface SavePickerWindow extends Window {
  showSaveFilePicker?: (options: {
    suggestedName: string
    types: Array<{ description: string; accept: Record<string, string[]> }>
  }) => Promise<SaveFileHandle>
}

function responseFileName(response: Response, fallback: string): string {
  const disposition = response.headers.get('content-disposition') ?? ''
  return /filename="?([^";]+)"?/i.exec(disposition)?.[1] ?? fallback
}

function buildURL(archiveID: number, params: Record<string, string | number | boolean>): string {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => query.set(key, String(value)))
  return `/api/v1/chat-archives/${archiveID}/download?${query.toString()}`
}

async function request(url: string): Promise<Response> {
  const headers = new Headers()
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const response = await fetch(url, { headers })
  if (!response.ok) {
    const body = await response.text()
    throw new Error(body || `下载失败（HTTP ${response.status}）`)
  }
  return response
}

export async function downloadArchivePackage(
  archiveID: number,
  messageFormat: 'jsonl' | 'csv' | 'md',
  suggestedName: string,
  includeService: boolean,
): Promise<string | null | undefined> {
  const picker = (window as SavePickerWindow).showSaveFilePicker
  if (!picker) return null

  let handle: SaveFileHandle
  try {
    handle = await picker({
      suggestedName,
      types: [{ description: 'ZIP 归档', accept: { 'application/zip': ['.zip'] } }],
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return undefined
    throw error
  }

  const response = await request(buildURL(archiveID, {
    format: 'zip', message_format: messageFormat, include_service: includeService,
  }))
  if (!response.body) throw new Error('浏览器不支持流式文件下载')
  const writable = await handle.createWritable()
  await response.body.pipeTo(writable)
  return responseFileName(response, suggestedName)
}
