import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import Icon from '../../components/Icon'
import { copyStoredFilePath } from './fileActions'
import type { StoredFile } from './filesSlice'
import FilePermissions from '../permissions/FilePermissions'
import type { SurfaceOrigin } from '../../components/surfaceAnimation'
import { useSurfaceTransition } from '../../components/useSurfaceTransition'
import { useDialogViewport } from '../../components/useDialogViewport'

export type CopyStatus = 'idle' | 'copying' | 'copied' | 'failed'

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
  useSurfaceTransition(dialog, open, origin, () => dialog.current?.close(), true)
  useDialogViewport(dialog)

  useEffect(() => {
    dialog.current?.querySelector<HTMLButtonElement>('.permissions-close-button')?.focus({ preventScroll: true })
  }, [])
  const [retryStatus, setRetryStatus] = useState<CopyStatus | null>(null)
  const status = retryStatus ?? initialStatus
  const url = new URL(`/files/${encodeURIComponent(file.id)}`, window.location.origin).href

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

  const content = <section className="share-link-section" aria-labelledby="share-link-title">
    <div className="share-section-heading"><h3 id="share-link-title">Ссылка на файл</h3></div>
    <div className="share-link-control form-field">
      <label className="visually-hidden" htmlFor="share-file-url">Ссылка на файл</label>
      <input ref={input} className="p-field" id="share-file-url" value={url} readOnly onFocus={(event) => event.currentTarget.select()} aria-describedby="share-copy-status" />
      <button className="p-button p-button-borderless p-button-icon-only share-copy-button" type="button" aria-label="Скопировать ссылку" title="Скопировать ссылку" onClick={() => void copy()} disabled={status === 'copying'}><Icon name="copy" /></button>
    </div>
    <p id="share-copy-status" role="status" aria-live="polite" className={`field-hint share-copy-status${status === 'failed' ? ' share-copy-status--failed' : ''}`}>{status === 'copied' ? 'Ссылка скопирована — можно отправить её получателю.' : status === 'copying' ? 'Копируем ссылку…' : status === 'failed' ? 'Не удалось скопировать. Выделите ссылку и выберите «Копировать».' : 'Открыть ссылку смогут только пользователи с доступом.'}</p>
  </section>

  if (canManagePermissions) return <FilePermissions file={file} token={token} origin={origin} onClose={onClose}>{content}</FilePermissions>

  return createPortal(<dialog ref={dialog} className="permissions-dialog" data-closing={!open} aria-modal="true" aria-labelledby="share-title" onClose={onClose} onCancel={(event) => { event.preventDefault(); setOpen(false) }}>
    <header className="permissions-dialog-header">
      <div><h2 id="share-title">{file.name}</h2><p>Права доступа</p></div>
      <button className="p-button p-button-secondary p-button-icon-only permissions-close-button" type="button" aria-label="Закрыть" autoFocus disabled={!open} onClick={() => setOpen(false)}><Icon name="close" /></button>
    </header>
    <div className="permissions-dialog-body">
      {content}
      <p className="permissions-owner-note"><Icon name="lock" /><span>Управлять доступом может только владелец файла.</span></p>
    </div>
  </dialog>, document.body)
}
