import { serializeApiError } from '../../api/client'

export interface UploadQueueState {
  total: number
  completed: number
  failed: number
  pending: number
  errors: { filename: string; message: string }[]
}

export const emptyUploadQueue: UploadQueueState = {
  total: 0, completed: 0, failed: 0, pending: 0, errors: [],
}

// Keep File objects outside Redux and send one file at a time. Each file's
// parts still upload concurrently, without multiplying requests for large drops.
export function createUploadQueue(
  upload: (file: File, signal: AbortSignal) => Promise<unknown>,
  onChange: (state: UploadQueueState) => void,
) {
  const files: File[] = []
  const controller = new AbortController()
  let running = false
  let state = { ...emptyUploadQueue }

  async function drain() {
    running = true
    while (files.length > 0 && !controller.signal.aborted) {
      const file = files.shift()!
      let error: { filename: string; message: string } | null = null
      try {
        await upload(file, controller.signal)
      } catch (cause) {
        error = { filename: file.name, message: serializeApiError(cause).message }
      }
      if (controller.signal.aborted) break
      state = {
        ...state,
        completed: state.completed + (error ? 0 : 1),
        failed: state.failed + (error ? 1 : 0),
        pending: state.pending - 1,
        errors: error ? [...state.errors, error] : state.errors,
      }
      onChange(state)
    }
    running = false
  }

  return {
    add(incoming: File[]) {
      if (incoming.length === 0 || controller.signal.aborted) return
      if (!running) state = { ...emptyUploadQueue }
      for (const file of incoming) files.push(file)
      state = { ...state, total: state.total + incoming.length, pending: state.pending + incoming.length }
      onChange(state)
      if (!running) void drain()
    },
    stop() {
      files.length = 0
      controller.abort()
    },
  }
}
