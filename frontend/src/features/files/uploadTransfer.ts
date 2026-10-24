import { apiRequest, apiUploadBuffer, serializeApiError } from '../../api/client'
import type { StoredFile } from './filesSlice'

export type UploadPhase = 'initializing' | 'reading' | 'transferring' | 'finalizing'

interface UploadOptions {
  file: File
  ownerId: string
  token: string | null
  signal: AbortSignal
  onPhase: (phase: UploadPhase) => void
  onPart: (current: number, total: number) => void
  onProgress: (confirmedBytes: number) => void
  onTransferProgress?: (sentBytes: number) => void
}

interface UploadStatus {
  upload: { file_id: string; size: number; parts: { position: number; size: number }[] }
}

const ATTEMPTS = 3
const POLL_INTERVAL = 1_000

function retryable(error: unknown): boolean {
  const { status } = serializeApiError(error)
  return status === undefined || status === 0 || status === 408 || status === 429 || status >= 500
}

// Race cancellation/deadlines explicitly as well as aborting the transport.
// Neither a missing network event nor a stuck file read can keep the queue open.
async function bounded<T>(
  operation: (signal: AbortSignal) => Promise<T>,
  parent: AbortSignal,
  timeout: number,
): Promise<T> {
  parent.throwIfAborted()
  const controller = new AbortController()
  const abort = () => controller.abort(parent.reason)
  parent.addEventListener('abort', abort, { once: true })
  const timer = setTimeout(() => controller.abort(new Error('Превышено время ожидания загрузки.')), timeout)
  let rejectAbort: () => void = () => {}
  const cancelled = new Promise<never>((_, reject) => {
    rejectAbort = () => reject(controller.signal.reason)
    controller.signal.addEventListener('abort', rejectAbort, { once: true })
  })
  try {
    return await Promise.race([operation(controller.signal), cancelled])
  } finally {
    clearTimeout(timer)
    parent.removeEventListener('abort', abort)
    controller.signal.removeEventListener('abort', rejectAbort)
    controller.abort()
  }
}

function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted()
    const abort = () => { clearTimeout(timer); reject(signal.reason) }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, ms)
    signal.addEventListener('abort', abort, { once: true })
  })
}

export async function uploadStoredFile(options: UploadOptions): Promise<StoredFile> {
  const { file, ownerId, token, signal, onPhase, onPart, onProgress } = options
  let uploadId: string | null = null
  function request<T>(path: string, init: RequestInit = {}, parent = signal, timeout = 30_000): Promise<T> {
    return bounded((requestSignal) => apiRequest<T>(path, { ...init, signal: requestSignal, cache: 'no-store' }, token), parent, timeout)
  }

  async function stored(position: number, size: number): Promise<boolean> {
    try {
      const { upload } = await request<UploadStatus>(`/v1/uploads/${uploadId}`, {}, signal, 10_000)
      if (upload.file_id !== uploadId || upload.size !== file.size) return false
      return upload.parts.some((part) => part.position === position && part.size === size)
    } catch {
      signal.throwIfAborted()
      return false
    }
  }

  // Observe the server while the PUT is still running. A committed part is an
  // acknowledgment even if the response to that PUT never arrives on the phone.
  async function sendPart(position: number, bytes: ArrayBuffer, offset: number): Promise<void> {
    signal.throwIfAborted()
    const attempt = new AbortController()
    const abort = () => attempt.abort(signal.reason)
    signal.addEventListener('abort', abort, { once: true })
    const sent = bounded((requestSignal) => apiUploadBuffer(`/v1/uploads/${uploadId}/parts/${position}`, bytes,
      requestSignal, (sentBytes) => {
        if (!signal.aborted) options.onTransferProgress?.(offset + sentBytes)
      }, token), attempt.signal, 600_000).then(
      () => ({ ok: true as const }),
      (error: unknown) => ({ ok: false as const, error }),
    )
    try {
      for (;;) {
        const result = await bounded((waitSignal) => Promise.race([
          sent,
          pause(POLL_INTERVAL, waitSignal).then(() => null),
        ]), signal, 610_000)
        signal.throwIfAborted()
        if (result?.ok) return
        if (result && !retryable(result.error)) throw result.error
        if (await stored(position, bytes.byteLength)) return
        if (result) throw result.error
      }
    } finally {
      signal.removeEventListener('abort', abort)
      attempt.abort()
      await sent
    }
  }

  try {
    onPhase('initializing')
    const initialized = await request<{ file: StoredFile; part_size: number }>('/v1/files/upload', {
      method: 'POST',
      body: JSON.stringify({ owner_id: ownerId, name: file.name, type: file.type || 'application/octet-stream', size: file.size }),
    })
    if (!initialized.file?.id) throw new Error('Сервер не вернул ID загрузки.')
    uploadId = initialized.file.id
    const partSize = initialized.part_size
    if (!Number.isSafeInteger(partSize) || partSize <= 0) throw new Error('Сервер вернул некорректный размер части файла.')
    const count = Math.ceil(file.size / partSize)
    onProgress(0)

    for (let position = 0; position < count; position++) {
      signal.throwIfAborted()
      onPart(position + 1, count)
      onPhase('reading')
      const start = position * partSize
      const end = Math.min(start + partSize, file.size)
      // Materialize one part only. Sending a plain buffer also avoids Safari's
      // file-backed Blob upload path; memory use stays bounded by part_size.
      const bytes = await bounded(() => file.slice(start, end).arrayBuffer(), signal, 60_000)
      onPhase('transferring')
      for (let attempt = 1; ; attempt++) {
        try {
          await sendPart(position, bytes, start)
          break
        } catch (error) {
          signal.throwIfAborted()
          if (!retryable(error) || attempt === ATTEMPTS) {
            throw new Error(`Не удалось сохранить часть ${position + 1} из ${count}. ${serializeApiError(error).message}`)
          }
          await pause(500 * attempt, signal)
        }
      }
      signal.throwIfAborted()
      onProgress(end)
    }

    onPhase('finalizing')
    for (let attempt = 1; ; attempt++) {
      try {
        const completed: { file: StoredFile } = await request(`/v1/uploads/${uploadId}/complete`, { method: 'POST' })
        const ready: StoredFile = completed.file
        if (ready.id !== uploadId || ready.status !== 'ready' || ready.size !== file.size) {
          throw new Error('Сервер не подтвердил готовность файла.')
        }
        return ready
      } catch (error) {
        signal.throwIfAborted()
        // A completion response can be lost after the transaction commits too.
        try {
          const response: { file: StoredFile } = await request(`/v1/files/${uploadId}`)
          const ready: StoredFile = response.file
          if (ready.id === uploadId && ready.status === 'ready' && ready.size === file.size) return ready
        } catch {
          signal.throwIfAborted()
        }
        if (!retryable(error) || attempt === ATTEMPTS) throw error
        await pause(500 * attempt, signal)
      }
    }
  } catch (error) {
    const cause = serializeApiError(error)
    if (uploadId) {
      try {
        // Cleanup must outlive cancellation of the UI request, but stay bounded.
        await request(`/v1/uploads/${uploadId}`, { method: 'DELETE' }, new AbortController().signal, 10_000)
      } catch (cleanupError) {
        cause.message += ` Не удалось подтвердить отмену: ${serializeApiError(cleanupError).message}`
      }
    }
    throw new Error(cause.message)
  }
}
