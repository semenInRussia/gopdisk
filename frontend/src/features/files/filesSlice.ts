import { createAsyncThunk, createEntityAdapter, createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { apiRequest, apiUploadPart, serializeApiError, type ApiError } from '../../api/client'
import type { AppDispatch, RootState } from '../../app/store'

export type FileStatus = 'uploading' | 'ready'
export type FileViewMode = 'list' | 'grid'

export interface StoredFile {
  id: string
  name: string
  type: string
  owner_id: string
  status: FileStatus
  size: number
}

interface FilesEnvelope {
  files: StoredFile[]
}

interface FileEnvelope {
  file: StoredFile
}

interface UploadInitializationEnvelope extends FileEnvelope {
  part_size: number
}

interface UploadState {
  status: 'idle' | 'uploading' | 'succeeded' | 'failed'
  filename: string | null
  uploadedBytes: number
  totalBytes: number
  error: ApiError | null
}

const UPLOAD_CONCURRENCY = 3

const filesAdapter = createEntityAdapter<StoredFile>({
  sortComparer: (left, right) => left.name.localeCompare(right.name),
})

const emptyUpload: UploadState = {
  status: 'idle',
  filename: null,
  uploadedBytes: 0,
  totalBytes: 0,
  error: null,
}

const initialState = filesAdapter.getInitialState({
  viewMode: 'list' as FileViewMode,
  selectedFileId: null as string | null,
  listStatus: 'idle' as 'idle' | 'loading' | 'succeeded' | 'failed',
  listError: null as ApiError | null,
  upload: emptyUpload,
})

export const loadFiles = createAsyncThunk<
  StoredFile[],
  string,
  { state: RootState; rejectValue: ApiError }
>('files/loadFiles', async (ownerId, { getState, rejectWithValue, signal }) => {
  try {
    const response = await apiRequest<FilesEnvelope>(
      `/v1/users/${ownerId}/files`,
      { signal },
      getState().auth.token,
    )
    return response.files
  } catch (error) {
    return rejectWithValue(serializeApiError(error))
  }
})

export const uploadFile = createAsyncThunk<
  StoredFile,
  { file: File; ownerId: string },
  { state: RootState; dispatch: AppDispatch; rejectValue: ApiError }
>('files/uploadFile', async ({ file, ownerId }, { dispatch, getState, rejectWithValue, signal }) => {
  const token = getState().auth.token
  const controller = new AbortController()
  const abortUpload = () => controller.abort()
  signal.addEventListener('abort', abortUpload, { once: true })

  let uploadId: string | null = null
  let workers: Promise<void>[] = []

  try {
    const initialized = await apiRequest<UploadInitializationEnvelope>('/v1/files/upload', {
      method: 'POST',
      body: JSON.stringify({
        owner_id: ownerId,
        name: file.name,
        type: file.type || 'application/octet-stream',
        size: file.size,
      }),
      signal: controller.signal,
    }, token)
    uploadId = initialized.file.id

    if (!Number.isSafeInteger(initialized.part_size) || initialized.part_size <= 0) {
      throw new Error('Сервер вернул некорректный размер части файла.')
    }

    const partSize = initialized.part_size
    const partCount = Math.ceil(file.size / partSize)
    let nextPosition = 0

    const uploadWorker = async () => {
      while (nextPosition < partCount) {
        const position = nextPosition
        nextPosition += 1
        const start = position * partSize
        const part = file.slice(start, Math.min(start + partSize, file.size))

        let sentBytes = 0
        await apiUploadPart(`/v1/uploads/${uploadId}/parts/${position}`, part, controller.signal, (bytes) => {
          const delta = Math.max(0, bytes - sentBytes)
          sentBytes += delta
          if (delta > 0) dispatch(uploadProgressed(delta))
        }, token)
      }
    }

    workers = Array.from(
      { length: Math.min(UPLOAD_CONCURRENCY, partCount) },
      () => uploadWorker(),
    )
    await Promise.all(workers)

    const completed = await apiRequest<FileEnvelope>(`/v1/uploads/${uploadId}/complete`, {
      method: 'POST',
      signal: controller.signal,
    }, token)
    return completed.file
  } catch (error) {
    controller.abort()
    await Promise.allSettled(workers)

    if (uploadId) {
      try {
        await apiRequest(`/v1/uploads/${uploadId}`, { method: 'DELETE' }, token)
      } catch {
        // Preserve the upload error. An unfinished session can still be resumed or removed later.
      }
    }
    return rejectWithValue(serializeApiError(error))
  } finally {
    signal.removeEventListener('abort', abortUpload)
  }
})

const filesSlice = createSlice({
  name: 'files',
  initialState,
  reducers: {
    fileViewModeChanged(state, action: PayloadAction<FileViewMode>) {
      state.viewMode = action.payload
    },
    filesReceived: filesAdapter.setAll,
    fileUpserted: filesAdapter.upsertOne,
    fileRemoved(state, action: PayloadAction<string>) {
      filesAdapter.removeOne(state, action.payload)
      if (state.selectedFileId === action.payload) state.selectedFileId = null
    },
    fileSelected(state, action: PayloadAction<string | null>) {
      state.selectedFileId = action.payload
    },
    filesCleared(state) {
      filesAdapter.removeAll(state)
      state.selectedFileId = null
      state.listStatus = 'idle'
      state.listError = null
      state.upload = { ...emptyUpload }
    },
    uploadProgressed(state, action: PayloadAction<number>) {
      state.upload.uploadedBytes = Math.min(
        state.upload.uploadedBytes + action.payload,
        state.upload.totalBytes,
      )
    },
    uploadFeedbackCleared(state) {
      if (state.upload.status !== 'uploading') state.upload = { ...emptyUpload }
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(loadFiles.pending, (state) => {
        state.listStatus = 'loading'
        state.listError = null
      })
      .addCase(loadFiles.fulfilled, (state, action) => {
        filesAdapter.setAll(state, action.payload)
        state.listStatus = 'succeeded'
      })
      .addCase(loadFiles.rejected, (state, action) => {
        state.listStatus = 'failed'
        state.listError = action.payload ?? { message: 'Не удалось загрузить список файлов.' }
      })
      .addCase(uploadFile.pending, (state, action) => {
        state.upload = {
          status: 'uploading',
          filename: action.meta.arg.file.name,
          uploadedBytes: 0,
          totalBytes: action.meta.arg.file.size,
          error: null,
        }
      })
      .addCase(uploadFile.fulfilled, (state, action) => {
        filesAdapter.upsertOne(state, action.payload)
        state.upload.status = 'succeeded'
        state.upload.uploadedBytes = state.upload.totalBytes
      })
      .addCase(uploadFile.rejected, (state, action) => {
        state.upload.status = 'failed'
        state.upload.error = action.payload ?? { message: 'Не удалось загрузить файл.' }
      })
  },
})

export const {
  fileViewModeChanged,
  fileRemoved,
  fileSelected,
  fileUpserted,
  filesCleared,
  filesReceived,
  uploadFeedbackCleared,
  uploadProgressed,
} = filesSlice.actions

const fileSelectors = filesAdapter.getSelectors((state: RootState) => state.files)

export const selectAllFiles = fileSelectors.selectAll
export const selectFileById = fileSelectors.selectById
export const selectFilesListStatus = (state: RootState) => state.files.listStatus
export const selectFilesListError = (state: RootState) => state.files.listError
export const selectUpload = (state: RootState) => state.files.upload
export const selectSelectedFile = (state: RootState) =>
  state.files.selectedFileId
    ? fileSelectors.selectById(state, state.files.selectedFileId)
    : null

export default filesSlice.reducer
