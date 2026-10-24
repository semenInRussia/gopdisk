import { useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import Icon from '../../components/Icon'
import { copyStoredFilePath } from './fileActions'
import type { StoredFile } from './filesSlice'
import FilePermissions from '../permissions/FilePermissions'
import type { SurfaceOrigin } from '../../components/surfaceAnimation'
import { useSurfaceTransition } from '../../components/useSurfaceTransition'

export type CopyStatus = 'copying' | 'copied' | 'failed'

interface Props {
  file: StoredFile
  initialStatus: CopyStatus
  token: string | null
  canManagePermissions: boolean
  origin: SurfaceOrigin
  onClose: () => void
}

export default function ShareFileDialog({ file, initialStatus, token, canManagePermissions, origin, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(true)
  useSurfaceTransition(dialog, open, origin, () => dialog.current?.close())
  const [retryStatus, setRetryStatus] = useState<CopyStatus | null>(null)
  const status = retryStatus ?? initialStatus
  const url = new URL(`/files/${encodeURIComponent(file.id)}`, window.location.origin).href

  useLayoutEffect(() => {
    input.current?.focus()
    input.current?.select()
  }, [origin])

  async function copy() {
    setRetryStatus('copying')
    try {
      await copyStoredFilePath(file.id, input.current ?? undefined)
      setRetryStatus('copied')
    } catch {
      setRetryStatus('failed')
      input.current?.focus()
      input.current?.select()
    }
  }

  const content = <>
    <div className="form-field share-file-field">
      <label className="form-label" htmlFor="share-file-url">Ссылка на файл</label>
      <input ref={input} className="p-field" id="share-file-url" value={url} readOnly onFocus={(event) => event.currentTarget.select()} aria-describedby="share-copy-status" />
    </div>
    <p id="share-copy-status" role="status" className="field-hint">{status === 'copied' ? 'Ссылка скопирована.' : status === 'copying' ? 'Копируем ссылку…' : 'Браузер не разрешил копирование. Ссылка выделена — нажмите ⌘C или Ctrl+C.'}</p>
    <p className="field-hint">Для открытия получателю нужны права доступа к файлу.</p>
    <div className="form-actions share-file-actions"><button className="p-button p-button-primary" type="button" onClick={() => void copy()}>Скопировать ссылку</button></div>
  </>

  if (canManagePermissions) return <FilePermissions file={file} token={token} origin={origin} onClose={onClose}>{content}</FilePermissions>

  return createPortal(<dialog ref={dialog} className="permissions-dialog" data-closing={!open} aria-labelledby="share-title" onClose={onClose} onCancel={(event) => { event.preventDefault(); setOpen(false) }}>
    <header className="permissions-dialog-header">
      <div><h2 id="share-title">Права доступа</h2><p>{file.name}</p></div>
      <button className="p-button p-button-secondary p-button-icon-only" type="button" aria-label="Закрыть" disabled={!open} onClick={() => setOpen(false)}><Icon name="close" /></button>
    </header>
    {content}
  </dialog>, document.body)
}
