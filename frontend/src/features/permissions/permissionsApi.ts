import { apiRequest } from '../../api/client'
import type { FilePermission, PermissionCode } from './permissionsSlice'

const path = (fileId: string) => `/v1/files/${encodeURIComponent(fileId)}/permissions`

export async function loadFilePermissions(fileId: string, token: string | null, signal?: AbortSignal): Promise<FilePermission[]> {
  const response = await apiRequest<{ permissions: FilePermission[] }>(path(fileId), { signal }, token)
  return response.permissions
}

export async function saveFilePermissions(fileId: string, userId: string, codes: PermissionCode[], token: string | null): Promise<void> {
  await apiRequest(`${path(fileId)}/${encodeURIComponent(userId)}`, {
    method: 'PUT', body: JSON.stringify({ codes }),
  }, token)
}

export async function revokeFilePermissions(fileId: string, userId: string, token: string | null): Promise<void> {
  await apiRequest(`${path(fileId)}/${encodeURIComponent(userId)}`, { method: 'DELETE' }, token)
}
