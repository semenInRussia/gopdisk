import { useEffect, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { useAppSelector } from '../../app/hooks'
import { apiRequest, serializeApiError } from '../../api/client'
import StatusMessage from '../auth/StatusMessage'
import { downloadStoredFile } from './fileActions'
import { openFile } from './openFile'
import type { StoredFile } from './filesSlice'

export default function SharedFile() {
  const { id = '' } = useParams()
  const { token, sessionStatus } = useAppSelector((state) => state.auth)
  const [file, setFile] = useState<StoredFile | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [downloading, setDownloading] = useState(false)

  useEffect(() => {
    if (sessionStatus !== 'authenticated') return
    const controller = new AbortController()
    setFile(null)
    setLoading(true)
    setError(null)
    apiRequest<{ file: StoredFile }>(`/v1/files/${encodeURIComponent(id)}`, { signal: controller.signal }, token).then((response) => {
      if (!controller.signal.aborted) setFile(response.file)
    }).catch((failure: unknown) => {
      if (!controller.signal.aborted) setError(serializeApiError(failure).message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [id, token, sessionStatus])

  if (sessionStatus !== 'authenticated') return <Navigate to={`/login?file=${encodeURIComponent(id)}`} replace />

  async function act(download: boolean) {
    if (!file) return
    setError(null)
    try {
      if (download) { setDownloading(true); await downloadStoredFile(file.id, file.name, token) }
      else await openFile(file.id, token)
    } catch (failure) {
      setError(serializeApiError(failure).message)
    } finally {
      setDownloading(false)
    }
  }

  return <main className="auth-card auth-card--compact">
    <header className="auth-header"><h1>{file?.name ?? 'Файл GopherDisk'}</h1><p>Доступ по ссылке</p></header>
    {loading && <p role="status">Загружаем файл…</p>}
    {error && <StatusMessage error={{ message: error }} />}
    {file && <div className="file-selection-actions">
      <button type="button" className="p-button p-button-primary" onClick={() => void act(false)}>Открыть файл</button>
      <button type="button" className="p-button p-button-secondary" disabled={downloading} onClick={() => void act(true)}>{downloading ? 'Скачиваем…' : 'Скачать'}</button>
    </div>}
    <Link className="text-link" to="/profile">К моим файлам</Link>
  </main>
}
