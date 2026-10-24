export interface ApiError {
  message: string
  status?: number
  fields?: Record<string, string>
}

class ApiRequestError extends Error {
  readonly status: number
  readonly fields?: Record<string, string>

  constructor(message: string, status: number, fields?: Record<string, string>) {
    super(message)
    this.name = 'ApiRequestError'
    this.status = status
    this.fields = fields
  }
}

async function apiResponse(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<Response> {
  const headers = new Headers(options.headers)
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const response = await fetch(path, { ...options, headers })
  if (!response.ok) {
    const isJSON = response.headers.get('content-type')?.includes('application/json')
    const body = isJSON ? await response.json() as unknown : null
    const error = (body as { error?: string | Record<string, string> } | null)?.error
    if (error && typeof error === 'object') {
      throw new ApiRequestError('Проверьте заполненные поля.', response.status, error)
    }
    throw new ApiRequestError(
      typeof error === 'string' ? error : 'Не удалось выполнить запрос.',
      response.status,
    )
  }

  return response
}

export async function apiRequest<T>(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<T> {
  const response = await apiResponse(path, options, token)
  if (response.status === 204) return undefined as T
  const isJSON = response.headers.get('content-type')?.includes('application/json')
  return (isJSON ? await response.json() : null) as T
}

export async function apiBlobRequest(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<Blob> {
  const response = await apiResponse(path, options, token)
  return response.blob()
}


// Use browser upload events for transfer progress. Server-side persistence is
// confirmed separately by the uploader, so a missing XHR load event is harmless.
export function apiUploadBuffer(
  path: string,
  bytes: ArrayBuffer,
  signal: AbortSignal,
  onProgress: (sentBytes: number) => void,
  token: string | null,
): Promise<void> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted()
    const xhr = new XMLHttpRequest()
    let settled = false
    const finish = (error?: unknown) => {
      if (settled) return
      settled = true
      signal.removeEventListener('abort', abort)
      xhr.upload.onprogress = null
      xhr.onload = xhr.onerror = xhr.onabort = xhr.ontimeout = null
      if (error !== undefined) reject(error)
      else resolve()
    }
    const abort = () => {
      finish(signal.reason ?? new DOMException('Загрузка отменена.', 'AbortError'))
      xhr.abort()
    }
    xhr.open('PUT', path)
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    xhr.upload.onprogress = (event) => {
      if (!signal.aborted && Number.isFinite(event.loaded) && event.loaded >= 0) {
        onProgress(Math.min(bytes.byteLength, event.loaded))
      }
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) { finish(); return }
      let message = 'Не удалось отправить часть файла.'
      try {
        const response = JSON.parse(xhr.responseText) as { error?: unknown }
        if (typeof response.error === 'string') message = response.error
      } catch { /* A proxy may return a non-JSON error. */ }
      finish(new ApiRequestError(message, xhr.status))
    }
    xhr.onerror = () => finish(new Error('Ошибка сети при передаче файла.'))
    xhr.onabort = () => finish(new DOMException('Загрузка отменена.', 'AbortError'))
    xhr.ontimeout = () => finish(new Error('Превышено время передачи части файла.'))
    signal.addEventListener('abort', abort, { once: true })
    try { xhr.send(bytes) } catch (error) { finish(error) }
  })
}

export function serializeApiError(error: unknown): ApiError {
  if (error instanceof ApiRequestError) {
    return { message: error.message, status: error.status, fields: error.fields }
  }
  if (error instanceof Error) return { message: error.message }
  return { message: 'Произошла неизвестная ошибка.' }
}
