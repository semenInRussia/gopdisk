import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import StatusMessage from '../auth/StatusMessage'
import { serializeApiError } from '../../api/client'
import { openFile } from './openFile'
import { createUploadQueue, emptyUploadQueue } from './uploadQueue'
import { listenForPageFileDrops } from './pageFileDrop'
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
  const queueRef = useRef<ReturnType<typeof createUploadQueue> | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const [queue, setQueue] = useState(emptyUploadQueue)
  const [dragging, setDragging] = useState(false)
  const [openError, setOpenError] = useState<string | null>(null)

  useEffect(() => {
    if (listStatus === 'idle') dispatch(loadFiles(ownerId))
  }, [dispatch, listStatus, ownerId])

  useEffect(() => {
    const uploadQueue = createUploadQueue(async (file, signal) => {
      const result = await dispatch(uploadFile({ file, ownerId }, { signal }))
      if (uploadFile.rejected.match(result)) {
        throw new Error(result.payload?.message ?? result.error.message ?? 'Не удалось загрузить файл.')
      }
    }, setQueue)
    setQueue(emptyUploadQueue)
    queueRef.current = uploadQueue
    return () => {
      uploadQueue.stop()
      queueRef.current = null
    }
  }, [dispatch, ownerId])

  useEffect(() => listenForPageFileDrops(window, (files) => {
    dispatch(uploadFeedbackCleared())
    queueRef.current?.add(files)
  }, setDragging), [dispatch, ownerId])

  function addFiles(files: File[]) {
    if (files.length === 0) return
    dispatch(uploadFeedbackCleared())
    queueRef.current?.add(files)
  }

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    addFiles(Array.from(event.target.files ?? []))
    event.target.value = ''
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
    <>
      {createPortal(
        <div className="floating-upload">
          <button
            className="floating-button"
            type="button"
            aria-label="Загрузить файлы"
            title="Загрузить файлы"
            onClick={() => inputRef.current?.click()}
          >
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="m12 3 9 5-9 5-9-5 9-5ZM3 8v9l9 5 9-5V8M12 13v9M7.5 5.5l9 5" />
            </svg>
          </button>
          <input ref={inputRef} type="file" multiple hidden onChange={handleFileChange} />
        </div>,
        document.body,
      )}
      {dragging && createPortal(
        <div className="page-drop-overlay" role="status" aria-live="polite">
          <div>
            <strong>Отпустите файлы для загрузки</strong>
            <p>Можно перетащить сразу несколько файлов</p>
          </div>
        </div>,
        document.body,
      )}
      <section
        className="files-section"
        aria-labelledby="files-heading"
      >
        <div className="section-heading">
          <div>
            <p className="eyebrow">Хранилище</p>
            <h2 id="files-heading">Мои файлы</h2>
          </div>
          <span className="files-count">{files.length}</span>
        </div>

        <p className="upload-hint">Перетащите файлы в любое место страницы или нажмите кнопку загрузки слева сверху.</p>

        {queue.total > 0 && (
          <p className="upload-summary" role="status" aria-live="polite">
            Загружено: {queue.completed} из {queue.total}
            {queue.pending > 0 && ` · Осталось: ${queue.pending}`}
            {queue.failed > 0 && ` · Ошибок: ${queue.failed}`}
          </p>
        )}

        {upload.status === 'uploading' && (
          <div className="upload-progress" role="status" aria-live="polite">
            <div className="upload-progress__track">
              <span style={{ width: `${progress}%` }} />
            </div>
            <p>{upload.filename}: {formatBytes(upload.uploadedBytes)} из {formatBytes(upload.totalBytes)} ({progress}%)</p>
          </div>
        )}
        {queue.errors.map((error, index) => (
          <StatusMessage key={index} error={{ message: `${error.filename}: ${error.message}` }} />
        ))}

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
    </>
  )
}
