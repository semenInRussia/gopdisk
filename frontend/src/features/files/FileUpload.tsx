import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import StatusMessage from '../auth/StatusMessage'
import { serializeApiError } from '../../api/client'
import { openFile } from './openFile'
import {
  loadFiles,
  selectAllFiles,
  selectFilesListError,
  selectFilesListStatus,
  selectUpload,
  uploadFeedbackCleared,
  uploadFile,
} from './filesSlice'

interface FileUploadProps {
  ownerId: string
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 Б'

  const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** unit
  return `${value.toLocaleString('ru-RU', { maximumFractionDigits: unit === 0 ? 0 : 1 })} ${units[unit]}`
}

export default function FileUpload({ ownerId }: FileUploadProps) {
  const dispatch = useAppDispatch()
  const token = useAppSelector((state) => state.auth.token)
  const files = useAppSelector(selectAllFiles)
  const listStatus = useAppSelector(selectFilesListStatus)
  const listError = useAppSelector(selectFilesListError)
  const upload = useAppSelector(selectUpload)
  const inputRef = useRef<HTMLInputElement>(null)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [openError, setOpenError] = useState<string | null>(null)

  useEffect(() => {
    if (listStatus === 'idle') dispatch(loadFiles(ownerId))
  }, [dispatch, listStatus, ownerId])

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    dispatch(uploadFeedbackCleared())
    setSelectedFile(event.target.files?.[0] ?? null)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedFile || upload.status === 'uploading') return

    const result = await dispatch(uploadFile({ file: selectedFile, ownerId }))
    if (uploadFile.fulfilled.match(result)) {
      setSelectedFile(null)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  async function handleOpenFile(id: string) {
    setOpenError(null)
    try {
      await openFile(id, token)
    } catch (error) {
      setOpenError(serializeApiError(error).message)
    }
  }

  const progress = upload.totalBytes === 0
    ? upload.status === 'succeeded' ? 100 : 0
    : Math.round((upload.uploadedBytes / upload.totalBytes) * 100)

  return (
    <section className="files-section" aria-labelledby="files-heading">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Хранилище</p>
          <h2 id="files-heading">Мои файлы</h2>
        </div>
        <span className="files-count">{files.length}</span>
      </div>

      <form className="upload-form" onSubmit={handleSubmit}>
        <label className="file-picker">
          <span>{selectedFile ? selectedFile.name : 'Выберите файл'}</span>
          <small>{selectedFile ? formatBytes(selectedFile.size) : 'Большие файлы будут загружены параллельными частями'}</small>
          <input
            ref={inputRef}
            type="file"
            onChange={handleFileChange}
            disabled={upload.status === 'uploading'}
          />
        </label>
        <button type="submit" disabled={!selectedFile || upload.status === 'uploading'}>
          {upload.status === 'uploading' ? `Загрузка ${progress}%` : 'Загрузить'}
        </button>
      </form>

      {upload.status === 'uploading' && (
        <div className="upload-progress" role="status" aria-live="polite">
          <div className="upload-progress__track">
            <span style={{ width: `${progress}%` }} />
          </div>
          <p>{upload.filename}: {formatBytes(upload.uploadedBytes)} из {formatBytes(upload.totalBytes)}</p>
        </div>
      )}
      <StatusMessage
        error={upload.error}
        message={upload.status === 'succeeded' ? 'Файл успешно загружен.' : null}
      />

      {listError && <StatusMessage error={listError} />}
      {openError && <StatusMessage error={{ message: openError }} />}
      {listStatus === 'loading' && <p className="files-placeholder">Загружаем список файлов…</p>}
      {listStatus === 'succeeded' && files.length === 0 && (
        <p className="files-placeholder">Здесь пока нет файлов.</p>
      )}
      {files.length > 0 && (
        <ul className="files-list">
          {files.map((file) => (
            <li key={file.id}>
              <button
                className="file-open"
                type="button"
                onClick={() => handleOpenFile(file.id)}
                aria-label={`Открыть ${file.name} в новой вкладке`}
              >
                <span className="file-icon" aria-hidden="true">↗</span>
                <span className="file-details">
                  <strong>{file.name}</strong>
                  <small>{file.type} · {formatBytes(file.size)}</small>
                </span>
                <span className="file-status">Готов</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
