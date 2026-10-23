import { createEntityAdapter, createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { RootState } from '../../app/store'

export type FileStatus = 'uploading' | 'ready'

export interface StoredFile {
  id: string
  name: string
  type: string
  owner_id: string
  status: FileStatus
  size: number
}

const filesAdapter = createEntityAdapter<StoredFile>({
  sortComparer: (left, right) => left.name.localeCompare(right.name),
})

const initialState = filesAdapter.getInitialState({
  selectedFileId: null as string | null,
})

const filesSlice = createSlice({
  name: 'files',
  initialState,
  reducers: {
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
    },
  },
})

export const { fileRemoved, fileSelected, fileUpserted, filesCleared, filesReceived } =
  filesSlice.actions

const fileSelectors = filesAdapter.getSelectors((state: RootState) => state.files)

export const selectAllFiles = fileSelectors.selectAll
export const selectFileById = fileSelectors.selectById
export const selectSelectedFile = (state: RootState) =>
  state.files.selectedFileId
    ? fileSelectors.selectById(state, state.files.selectedFileId)
    : null

export default filesSlice.reducer
