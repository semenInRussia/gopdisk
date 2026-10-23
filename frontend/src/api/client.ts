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

export async function apiRequest<T>(
  path: string,
  options: RequestInit = {},
  token?: string | null,
): Promise<T> {
  const headers = new Headers(options.headers)
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const response = await fetch(path, { ...options, headers })
  if (response.status === 204) return undefined as T

  const isJSON = response.headers.get('content-type')?.includes('application/json')
  const body = isJSON ? await response.json() as unknown : null

  if (!response.ok) {
    const error = (body as { error?: string | Record<string, string> } | null)?.error
    if (error && typeof error === 'object') {
      throw new ApiRequestError('Проверьте заполненные поля.', response.status, error)
    }
    throw new ApiRequestError(
      typeof error === 'string' ? error : 'Не удалось выполнить запрос.',
      response.status,
    )
  }

  return body as T
}

export function serializeApiError(error: unknown): ApiError {
  if (error instanceof ApiRequestError) {
    return { message: error.message, status: error.status, fields: error.fields }
  }
  if (error instanceof Error) return { message: error.message }
  return { message: 'Произошла неизвестная ошибка.' }
}
