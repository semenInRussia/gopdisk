import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import Icon, { type IconName } from '../../components/Icon'
import StatusMessage from '../auth/StatusMessage'
import { serializeApiError } from '../../api/client'
import { openFile } from './openFile'
import { createUploadQueue, emptyUploadQueue } from './uploadQueue'
import { listenForPageFileDrops } from './pageFileDrop'
import {
  fileViewModeChanged,
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

function describeFile(type: string, name: string): { icon: IconName; label: string } {
  if (type.startsWith('image/')) return { icon: 'image', label: 'Изображение' }
  if (type.startsWith('video/')) return { icon: 'video', label: 'Видео' }
  if (type.startsWith('audio/')) return { icon: 'audio', label: 'Аудио' }
  if (type === 'application/pdf') return { icon: 'file', label: 'Документ PDF' }
  if (type.startsWith('text/')) return { icon: 'file', label: 'Текстовый документ' }
  const extension = name.includes('.') ? name.split('.').pop()?.toUpperCase() : null
  return { icon: 'file', label: extension ? `Файл ${extension}` : 'Файл' }
}

export default function FileUpload({ ownerId }: FileUploadProps) {
  const dispatch = useAppDispatch()
  const token = useAppSelector((state) => state.auth.token)
  const files = useAppSelector(selectAllFiles)
  const listStatus = useAppSelector(selectFilesListStatus)
  const listError = useAppSelector(selectFilesListError)
  const upload = useAppSelector(selectUpload)
  const viewMode = useAppSelector((state) => state.files.viewMode)
  const queueRef = useRef<ReturnType<typeof createUploadQueue> | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const [queue, setQueue] = useState(emptyUploadQueue)
  const [dragging, setDragging] = useState(false)
  const [openError, setOpenError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const uploading = queue.pending > 0
  const visibleFiles = files.filter((file) => file.name.toLocaleLowerCase('ru-RU').includes(search.trim().toLocaleLowerCase('ru-RU')))

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
    setDragging(false)
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
            className="p-button p-button-glass p-button-icon-only floating-button compose-button"
            type="button"
            aria-label="Загрузить файлы"
            title="Загрузить файлы"
            onClick={() => inputRef.current?.click()}
          >
            <Icon name="compose" />
          </button>
          <input ref={inputRef} type="file" multiple hidden onChange={handleFileChange} />
        </div>,
        document.body,
      )}
      {dragging && !uploading && createPortal(
        <div className="p-material-thick page-drop-overlay" role="status" aria-live="polite">
          <div>
            <Icon name="box" />
            <strong>Отпустите файлы для загрузки</strong>
            <p>Можно перетащить сразу несколько файлов</p>
          </div>
        </div>,
        document.body,
      )}
      <section
        className="files-section"
        aria-label="Файлы"
      >
        <div className="p-glass files-toolbar">
          <div className="files-heading">
            <h2>Все файлы</h2>
            <span className="p-glass-sm file-count" aria-label={`Всего файлов: ${files.length}`}>{files.length}</span>
          </div>
          <div className="files-toolbar-actions">
            <label className="file-search">
              <Icon name="search" />
              <span className="visually-hidden">Поиск файлов по имени</span>
              <input className="p-field" type="search" placeholder="Поиск файлов" value={search} onChange={(event) => setSearch(event.target.value)} />
            </label>
            <div className="p-segmented p-segmented-large file-view-control" role="group" aria-label="Вид файлов">
              <label className="p-segment">
                <input type="radio" name="file-view" value="list" checked={viewMode === 'list'} onChange={() => dispatch(fileViewModeChanged('list'))} />
                <span>Список</span>
              </label>
              <label className="p-segment">
                <input type="radio" name="file-view" value="grid" checked={viewMode === 'grid'} onChange={() => dispatch(fileViewModeChanged('grid'))} />
                <span>Сетка</span>
              </label>
            </div>
          </div>
        </div>

        {queue.total > 0 && !uploading && (
          <p className="upload-summary" role="status" aria-live="polite">
            Загружено: {queue.completed} из {queue.total}
            {queue.pending > 0 && ` · Осталось: ${queue.pending}`}
            {queue.failed > 0 && ` · Ошибок: ${queue.failed}`}
          </p>
        )}

        {uploading && createPortal(
          <aside className="p-glass upload-progress" role="status" aria-live="polite" aria-label="Загрузка файлов">
            <strong>Загрузка файлов · {progress}%</strong>
            <progress className="p-progress" max={100} value={progress} aria-label={`Загрузка ${upload.filename ?? 'файла'}`} />
            <p>{upload.filename}: {formatBytes(upload.uploadedBytes)} из {formatBytes(upload.totalBytes)}</p>
            <p>{progress === 100 ? 'Сохраняем файл… · ' : ''}Загружено: {queue.completed} из {queue.total}{queue.failed > 0 && ` · Ошибок: ${queue.failed}`}</p>
          </aside>,
          document.body,
        )}
        {queue.errors.map((error, index) => (
          <StatusMessage key={index} error={{ message: `${error.filename}: ${error.message}` }} />
        ))}

        {listError && <StatusMessage error={listError} />}
        {openError && <StatusMessage error={{ message: openError }} />}
        {listStatus === 'failed' && (
          <button className="p-button p-button-glass p-button-touch-large" type="button" onClick={() => dispatch(loadFiles(ownerId))}>Повторить загрузку списка</button>
        )}
        {listStatus === 'loading' && <p className="files-placeholder" role="status"><span className="p-spinner p-spinner-small" aria-hidden="true" /> Загружаем файлы…</p>}
        {listStatus === 'succeeded' && files.length === 0 && !uploading && (
          <div className="p-glass files-empty">
            <Icon name="box" />
            <h2>Здесь пока нет файлов</h2>
            <p>Перетащите файлы сюда или нажмите кнопку с карандашом справа внизу.</p>
          </div>
        )}
        {files.length > 0 && visibleFiles.length === 0 && (
          <div className="p-glass files-empty" role="status">
            <Icon name="search" />
            <h2>Ничего не найдено</h2>
            <p>Попробуйте другое название файла.</p>
            <button className="p-button p-button-glass" type="button" onClick={() => setSearch('')}>Сбросить поиск</button>
          </div>
        )}
        {visibleFiles.length > 0 && (
          <ul className={`files-list${viewMode === 'grid' ? ' p-glass files-list--grid' : ''}`}>
            {visibleFiles.map((file) => {
              const presentation = describeFile(file.type, file.name)
              return (
                <li key={file.id}>
                  <button
                    className={viewMode === 'grid' ? 'file-open' : 'p-glass file-open'}
                    type="button"
                    onClick={() => handleOpenFile(file.id)}
                    aria-label={`Открыть ${file.name} в новой вкладке${viewMode === 'list' ? `, ${formatBytes(file.size)}` : ''}`}
                    title={`Открыть ${file.name} в новой вкладке`}
                  >
                    <span className={`${viewMode === 'list' ? 'p-glass-sm ' : ''}file-icon file-icon--${presentation.icon}`}><Icon name={presentation.icon} /></span>
                    <span className="file-details">
                      <strong>{file.name}</strong>
                      {viewMode === 'list' && <small>{presentation.label}</small>}
                    </span>
                    {viewMode === 'list' && <span className="file-size">{formatBytes(file.size)}</span>}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
        {queue.total === 0 && <p className="upload-hint"><Icon name="compose" /> Добавьте файлы кнопкой справа внизу или перетащите их сюда.</p>}
      </section>
    </>
  )
}
