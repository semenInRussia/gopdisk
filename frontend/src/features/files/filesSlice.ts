import { createAsyncThunk, createEntityAdapter, createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { apiRequest, serializeApiError, type ApiError } from '../../api/client'
import type { AppDispatch, RootState } from '../../app/store'
import { uploadStoredFile, type UploadPhase } from './uploadTransfer'

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

interface UploadState {
  status: 'idle' | 'uploading' | 'succeeded' | 'failed'
  phase: UploadPhase
  currentPart: number
  totalParts: number
  filename: string | null
  uploadedBytes: number
  sentBytes: number
  totalBytes: number
  error: ApiError | null
}

const filesAdapter = createEntityAdapter<StoredFile>({
  sortComparer: (left, right) => left.name.localeCompare(right.name),
})

const emptyUpload: UploadState = {
  status: 'idle',
  phase: 'initializing',
  currentPart: 0,
  totalParts: 0,
  filename: null,
  uploadedBytes: 0,
  sentBytes: 0,
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
  try {
    return await uploadStoredFile({
      file, ownerId, token: getState().auth.token, signal,
      onPhase: (phase) => { if (!signal.aborted) dispatch(uploadPhaseChanged(phase)) },
      onPart: (currentPart, totalParts) => { if (!signal.aborted) dispatch(uploadPartChanged({ currentPart, totalParts })) },
      onProgress: (bytes) => { if (!signal.aborted) dispatch(uploadConfirmed(bytes)) },
      onTransferProgress: (bytes) => { if (!signal.aborted) dispatch(uploadSent(bytes)) },
    })
  } catch (error) {
    return rejectWithValue(serializeApiError(error))
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
    uploadConfirmed(state, action: PayloadAction<number>) {
      state.upload.uploadedBytes = Math.min(Math.max(state.upload.uploadedBytes, action.payload), state.upload.totalBytes)
      state.upload.sentBytes = Math.max(state.upload.sentBytes, state.upload.uploadedBytes)
    },
    uploadSent(state, action: PayloadAction<number>) {
      state.upload.sentBytes = Math.min(Math.max(state.upload.sentBytes, action.payload), state.upload.totalBytes)
    },
    uploadPartChanged(state, action: PayloadAction<{ currentPart: number; totalParts: number }>) {
      state.upload.currentPart = action.payload.currentPart
      state.upload.totalParts = action.payload.totalParts
    },
    uploadPhaseChanged(state, action: PayloadAction<UploadState['phase']>) {
      state.upload.phase = action.payload
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
          phase: 'initializing',
          currentPart: 0,
          totalParts: 0,
          filename: action.meta.arg.file.name,
          uploadedBytes: 0,
          sentBytes: 0,
          totalBytes: action.meta.arg.file.size,
          error: null,
        }
      })
      .addCase(uploadFile.fulfilled, (state, action) => {
        filesAdapter.upsertOne(state, action.payload)
        state.upload.status = 'succeeded'
        state.upload.uploadedBytes = state.upload.totalBytes
        state.upload.sentBytes = state.upload.totalBytes
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
  uploadConfirmed,
  uploadSent,
  uploadPartChanged,
  uploadPhaseChanged,
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
