import { apiBlobRequest, apiRequest } from '../../api/client'

export async function copyStoredFilePath(id: string, selectedField?: HTMLInputElement): Promise<void> {
  const url = new URL(`/files/${encodeURIComponent(id)}`, window.location.origin)
  if (selectedField) {
    selectedField.focus({ preventScroll: true })
    selectedField.select()
    selectedField.setSelectionRange(0, selectedField.value.length)
    // Run within the original click, before an async permission check can expire it.
    if (typeof document.execCommand === 'function' && document.execCommand('copy')) return
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url.href)
      return
    }
  } catch {
    // Some browsers deny the async clipboard while allowing a copy from a selection.
  }
  const previous = document.activeElement as HTMLElement | null
  const field = document.createElement('textarea')
  field.value = url.href
  field.readOnly = true
  field.style.position = 'fixed'
  field.style.opacity = '0'
  field.style.top = '0'
  field.style.left = '0'
  const container = document.querySelector?.('dialog[open]') ?? document.body
  container.append(field)
  try {
    field.focus({ preventScroll: true })
    field.select()
    field.setSelectionRange(0, field.value.length)
    if (typeof document.execCommand !== 'function' || !document.execCommand('copy')) {
      throw new Error('Не удалось скопировать ссылку. Разрешите доступ к буферу обмена в браузере.')
    }
  } finally {
    field.remove()
    previous?.focus({ preventScroll: true })
  }
}

export async function deleteStoredFile(id: string, token: string | null): Promise<void> {
  await apiRequest(`/v1/files/${encodeURIComponent(id)}`, { method: 'DELETE' }, token)
}

export async function downloadStoredFile(id: string, name: string, token: string | null): Promise<void> {
  const content = await apiBlobRequest(`/v1/files/${encodeURIComponent(id)}/content`, {}, token)
  saveDownload(content, name)
}

export async function downloadStoredFiles(files: { id: string; name: string }[], token: string | null, onProgress?: (completed: number, total: number) => void): Promise<void> {
  if (files.length === 0) return
  if (files.length === 1) return downloadStoredFile(files[0].id, files[0].name, token)
  const { createFileArchive } = await import('./fileArchive')
  const archive = await createFileArchive(files, token, onProgress)
  saveDownload(archive, 'GopherDisk.zip')
}

function saveDownload(content: Blob, name: string): void {
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
