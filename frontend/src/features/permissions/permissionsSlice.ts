import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { RootState } from '../../app/store'

export type PermissionCode = 'files:read' | 'files:write' | 'files:delete'

export interface FilePermission {
  file_id: string
  user_id: string
  code: PermissionCode
}

interface PermissionsState {
  byFileId: Record<string, FilePermission[]>
}

const initialState: PermissionsState = {
  byFileId: {},
}

const permissionsSlice = createSlice({
  name: 'permissions',
  initialState,
  reducers: {
    permissionsReceived(
      state,
      action: PayloadAction<{ fileId: string; permissions: FilePermission[] }>,
    ) {
      state.byFileId[action.payload.fileId] = action.payload.permissions
    },
    permissionGranted(state, action: PayloadAction<FilePermission>) {
      const permission = action.payload
      const current = state.byFileId[permission.file_id] ?? []
      const alreadyExists = current.some(
        (item) => item.user_id === permission.user_id && item.code === permission.code,
      )
      if (!alreadyExists) current.push(permission)
      state.byFileId[permission.file_id] = current
    },
    permissionRevoked(state, action: PayloadAction<FilePermission>) {
      const permission = action.payload
      const current = state.byFileId[permission.file_id] ?? []
      state.byFileId[permission.file_id] = current.filter(
        (item) => item.user_id !== permission.user_id || item.code !== permission.code,
      )
    },
    userPermissionsReplaced(
      state,
      action: PayloadAction<{ fileId: string; userId: string; codes: PermissionCode[] }>,
    ) {
      const { fileId, userId, codes } = action.payload
      const otherUsers = (state.byFileId[fileId] ?? []).filter(
        (permission) => permission.user_id !== userId,
      )
      state.byFileId[fileId] = [
        ...otherUsers,
        ...codes.map((code) => ({ file_id: fileId, user_id: userId, code })),
      ]
    },
    permissionsClearedForFile(state, action: PayloadAction<string>) {
      delete state.byFileId[action.payload]
    },
    permissionsCleared() {
      return initialState
    },
  },
})

export const {
  permissionGranted,
  permissionRevoked,
  permissionsCleared,
  permissionsClearedForFile,
  permissionsReceived,
  userPermissionsReplaced,
} = permissionsSlice.actions

export const selectPermissionsForFile = (state: RootState, fileId: string) =>
  state.permissions.byFileId[fileId] ?? []

export const selectPermissionsForUser = (
  state: RootState,
  fileId: string,
  userId: string,
) => selectPermissionsForFile(state, fileId).filter((permission) => permission.user_id === userId)

export default permissionsSlice.reducer
