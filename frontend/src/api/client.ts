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

export function apiUploadPart(
  path: string,
  body: Blob,
  signal: AbortSignal,
  onProgress: (bytes: number) => void,
  token?: string | null,
): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Загрузка отменена.', 'AbortError'))
      return
    }
    const request = new XMLHttpRequest()
    const abort = () => request.abort()
    request.open('PUT', path)
    request.setRequestHeader('Content-Type', 'application/octet-stream')
    if (token) request.setRequestHeader('Authorization', `Bearer ${token}`)
    request.upload.onprogress = (event) => onProgress(Math.min(event.loaded, body.size))
    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        onProgress(body.size)
        resolve()
      } else {
        let message = 'Не удалось загрузить часть файла.'
        try {
          const response = JSON.parse(request.responseText) as { error?: unknown }
          if (typeof response.error === 'string') message = response.error
        } catch {
          // Non-JSON error responses use the default message.
        }
        reject(new ApiRequestError(message, request.status))
      }
    }
    request.onerror = () => reject(new Error('Ошибка сети при загрузке файла.'))
    request.onabort = () => reject(new DOMException('Загрузка отменена.', 'AbortError'))
    request.onloadend = () => signal.removeEventListener('abort', abort)
    signal.addEventListener('abort', abort, { once: true })
    request.send(body)
  })
}

export function serializeApiError(error: unknown): ApiError {
  if (error instanceof ApiRequestError) {
    return { message: error.message, status: error.status, fields: error.fields }
  }
  if (error instanceof Error) return { message: error.message }
  return { message: 'Произошла неизвестная ошибка.' }
}
