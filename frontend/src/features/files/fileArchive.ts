import { BlobReader, BlobWriter, ZipWriter } from '@zip.js/zip.js'
import { apiBlobRequest, serializeApiError } from '../../api/client'

export interface DownloadFile {
  id: string
  name: string
}

function archiveName(name: string, used: Set<string>): string {
  const base = (name.replaceAll('\\', '/').split('/').pop() ?? '')
    .replace(/[\u0000-\u001f<>:"|?*]/g, '_').replace(/[. ]+$/g, '') || 'file'
  const dot = base.lastIndexOf('.')
  const stem = dot > 0 ? base.slice(0, dot) : base
  const extension = dot > 0 ? base.slice(dot) : ''
  let candidate = base
  for (let suffix = 2; used.has(candidate.toLowerCase()); suffix++) candidate = `${stem} (${suffix})${extension}`
  used.add(candidate.toLowerCase())
  return candidate
}

export async function createFileArchive(files: DownloadFile[], token: string | null, onProgress?: (completed: number, total: number) => void): Promise<Blob> {
  const writer = new ZipWriter(new BlobWriter('application/zip'), { level: 0, useWebWorkers: false })
  const names = new Set<string>()
  try {
    onProgress?.(0, files.length)
    for (const [index, file] of files.entries()) {
      try {
        const content = await apiBlobRequest(`/v1/files/${encodeURIComponent(file.id)}/content`, {}, token)
        await writer.add(archiveName(file.name, names), new BlobReader(content))
      } catch (error) {
        throw new Error(`${file.name}: ${serializeApiError(error).message}`)
      }
      onProgress?.(index + 1, files.length)
    }
    return await writer.close()
  } catch (error) {
    try { await writer.close() } catch { /* Preserve the original file or archive failure. */ }
    throw error
  }
}
