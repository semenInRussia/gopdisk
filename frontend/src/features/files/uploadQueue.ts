import { serializeApiError } from '../../api/client'

export interface UploadQueueState {
  total: number
  completed: number
  failed: number
  pending: number
  cancelled: number
  errors: { filename: string; message: string }[]
}

export const emptyUploadQueue: UploadQueueState = {
  total: 0, completed: 0, failed: 0, pending: 0, cancelled: 0, errors: [],
}

// Keep File objects outside Redux and process files in selection order.
export function createUploadQueue(
  upload: (file: File, signal: AbortSignal) => Promise<unknown>,
  onChange: (state: UploadQueueState) => void,
) {
  const files: File[] = []
  let activeController: AbortController | null = null
  let stopped = false
  let running = false
  let state = { ...emptyUploadQueue }

  async function drain() {
    running = true
    while (files.length > 0 && !stopped) {
      const file = files.shift()!
      const controller = new AbortController()
      activeController = controller
      let error: { filename: string; message: string } | null = null
      try {
        await upload(file, controller.signal)
      } catch (cause) {
        error = { filename: file.name, message: serializeApiError(cause).message }
      }
      activeController = null
      if (controller.signal.aborted) continue
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
      if (incoming.length === 0 || stopped) return
      if (!running || activeController?.signal.aborted) state = { ...emptyUploadQueue }
      for (const file of incoming) files.push(file)
      state = { ...state, total: state.total + incoming.length, pending: state.pending + incoming.length }
      onChange(state)
      if (!running) void drain()
    },
    cancel() {
      files.length = 0
      activeController?.abort()
      state = { ...state, cancelled: state.cancelled + state.pending, pending: 0 }
      onChange(state)
    },
    stop() {
      stopped = true
      files.length = 0
      activeController?.abort()
    },
  }
}
