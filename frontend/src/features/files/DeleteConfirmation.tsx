import { useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { StoredFile } from './filesSlice'

export interface DeleteAnchor {
  element: HTMLElement
  offset?: { left: number; top: number; width: number; height: number }
}

interface Props {
  files: StoredFile[]
  anchor: DeleteAnchor | null
  onConfirm: () => void
  onCancel: () => void
}

export default function DeleteConfirmation({ files, anchor, onConfirm, onCancel }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)
  const result = useRef<'delete' | 'cancel' | null>(null)
  const [closing, setClosing] = useState(false)

  function requestClose(value: 'delete' | 'cancel') {
    if (result.current) return
    result.current = value
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) dialog.current?.close(value)
    else setClosing(true)
  }

  useLayoutEffect(() => {
    const element = dialog.current
    if (!element) return
    element.showModal()
    const position = () => {
      const width = element.offsetWidth
      const height = element.offsetHeight
      const viewportWidth = document.documentElement.clientWidth || window.innerWidth
      const viewportHeight = window.innerHeight
      const base = anchor?.element.isConnected ? anchor.element.getBoundingClientRect() : null
      const source = base && anchor?.offset ? {
        left: base.left + anchor.offset.left, right: base.left + anchor.offset.left + anchor.offset.width,
        top: base.top + anchor.offset.top, bottom: base.top + anchor.offset.top + anchor.offset.height,
      } : base
      const below = source && source.bottom + height <= viewportHeight
      const x = source ? source.right - width : (viewportWidth - width) / 2
      const y = source ? (below ? source.bottom : source.top - height) : (viewportHeight - height) / 2
      const left = Math.max(0, Math.min(x, viewportWidth - width))
      const top = Math.max(0, Math.min(y, viewportHeight - height))
      element.style.left = `${left}px`
      element.style.top = `${top}px`
      const originX = source ? (source.left + source.right) / 2 - left : width / 2
      const originY = source ? (below ? source.bottom : source.top) - top : height / 2
      element.style.transformOrigin = `${originX}px ${originY}px`
    }
    position()
    cancel.current?.focus({ preventScroll: true })
    let frame = 0
    const follow = () => {
      position()
      if (window.requestAnimationFrame) {
        window.cancelAnimationFrame(frame)
        frame = window.requestAnimationFrame(position)
      }
    }
    const observer = new ResizeObserver(position)
    observer.observe(element)
    if (anchor?.element.isConnected) observer.observe(anchor.element)
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    const finishClose = () => { if (reducedMotion.matches && result.current) element.close(result.current) }
    reducedMotion.addEventListener('change', finishClose)
    window.addEventListener('resize', position)
    window.addEventListener('scroll', follow, true)
    return () => {
      observer.disconnect()
      reducedMotion.removeEventListener('change', finishClose)
      window.removeEventListener('resize', position)
      window.removeEventListener('scroll', follow, true)
      if (frame) window.cancelAnimationFrame(frame)
    }
  }, [anchor])

  return createPortal(<dialog ref={dialog} id="file-delete-dialog" className="p-desktop-modal active file-delete-dialog" style={{ transform: 'none' }} data-closing={closing} role="alertdialog" aria-modal="true" aria-labelledby="delete-title" aria-describedby="delete-description"
    onAnimationEnd={(event) => { if (event.target === event.currentTarget && event.animationName === 'delete-dialog-exit' && result.current) dialog.current?.close(result.current) }}
    onCancel={(event) => { event.preventDefault(); requestClose('cancel') }}
    onClose={() => { if (dialog.current?.returnValue === 'delete') onConfirm(); else onCancel() }}>
    <div className="p-desktop-modal__surface">
      <div className="p-desktop-modal__body">
        <div className="p-desktop-modal__title" id="delete-title" role="heading" aria-level={2}>{files.length === 1 ? 'Удалить файл?' : `Удалить файлы (${files.length})?`}</div>
        <div className="p-desktop-modal__description" id="delete-description">{files.length === 1 ? `«${files[0].name}» будет удалён. ` : 'Выбранные файлы будут удалены. '}Это действие нельзя отменить.</div>
        {files.length > 1 && <div className="p-desktop-modal__description">{files.slice(0, 5).map((file) => file.name).join(', ')}{files.length > 5 && `; ещё файлов: ${files.length - 5}`}</div>}
      </div>
      <div className="p-desktop-modal__actions">
        <button ref={cancel} className="p-desktop-modal__action p-desktop-modal__action--secondary" type="button" disabled={closing} onClick={() => requestClose('cancel')}>Отмена</button>
        <button className="p-desktop-modal__action p-desktop-modal__action--destructive" type="button" disabled={closing} onClick={() => requestClose('delete')}>Удалить</button>
      </div>
    </div>
  </dialog>, document.body)
}
