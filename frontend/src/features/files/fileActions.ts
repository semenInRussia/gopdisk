import { apiBlobRequest, apiRequest } from '../../api/client'

export async function deleteStoredFile(id: string, token: string | null): Promise<void> {
  await apiRequest(`/v1/files/${encodeURIComponent(id)}`, { method: 'DELETE' }, token)
}

export async function downloadStoredFile(id: string, name: string, token: string | null): Promise<void> {
  const content = await apiBlobRequest(`/v1/files/${encodeURIComponent(id)}/content`, {}, token)
  const url = URL.createObjectURL(content)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  document.body.append(link)
  try {
    link.click()
  } finally {
    link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
  }
}
