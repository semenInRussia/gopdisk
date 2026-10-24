import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch } from '../../app/hooks'
import { serializeApiError } from '../../api/client'
import Icon from '../../components/Icon'
import { fileRemoved, type FileViewMode, type StoredFile } from './filesSlice'
import { describeFile, formatBytes } from './filePresentation'
import { deleteStoredFile, downloadStoredFile } from './fileActions'
import { intersects, selectFileIds } from './fileSelection'
import { openFile } from './openFile'

interface Props {
  files: StoredFile[]
  viewMode: FileViewMode
  token: string | null
  onError: (message: string | null) => void
}

export default function FileBrowser({ files, viewMode, token, onError }: Props) {
  const dispatch = useAppDispatch()
  const [selected, setSelected] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const [marquee, setMarquee] = useState<{ left: number; top: number; width: number; height: number } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const anchor = useRef<string | null>(null)
  const drag = useRef<{ x: number; y: number; base: string[] } | null>(null)
  const ids = files.map((file) => file.id)
  const selectedFiles = files.filter((file) => selected.includes(file.id))

  useEffect(() => {
    setSelected((current) => {
      const remaining = current.filter((id) => files.some((file) => file.id === id))
      return remaining.length === current.length ? current : remaining
    })
  }, [files])

  function closeMenu() {
    setMenu(null)
    if (returnFocus.current?.isConnected) returnFocus.current.focus()
    root.current?.focus()
  }

  function showMenu(id: string, element: HTMLButtonElement, x: number, y: number) {
    if (busy) return
    if (!selected.includes(id)) { setSelected([id]); anchor.current = id }
    returnFocus.current = element
    setMenu({ x: Math.max(8, Math.min(x, window.innerWidth - 208)), y: Math.max(8, Math.min(y, window.innerHeight - 112)) })
  }

  useEffect(() => {
    if (!menu) return
    menuRef.current?.querySelector<HTMLButtonElement>('button')?.focus()
    function outside(event: globalThis.PointerEvent) {
      if (event.target instanceof Node && !menuRef.current?.contains(event.target)) setMenu(null)
    }
    const dismiss = () => setMenu(null)
    document.addEventListener('pointerdown', outside)
    window.addEventListener('resize', dismiss)
    window.addEventListener('scroll', dismiss, true)
    return () => {
      document.removeEventListener('pointerdown', outside)
      window.removeEventListener('resize', dismiss)
      window.removeEventListener('scroll', dismiss, true)
    }
  }, [menu])

  async function runAction(action: 'download' | 'delete') {
    if (busy || selectedFiles.length === 0) return
    closeMenu()
    const targets = [...selectedFiles]
    if (action === 'delete' && !window.confirm(`Удалить файлов: ${targets.length}?\n${targets.slice(0, 5).map((file) => file.name).join('\n')}${targets.length > 5 ? '\n…' : ''}\nЭто действие нельзя отменить.`)) return
    setBusy(true)
    onError(null)
    const failures: string[] = []
    try {
      for (const file of targets) {
        try {
          if (action === 'delete') {
            await deleteStoredFile(file.id, token)
            dispatch(fileRemoved(file.id))
            setSelected((current) => current.filter((id) => id !== file.id))
          } else {
            await downloadStoredFile(file.id, file.name, token)
          }
        } catch (error) {
          failures.push(`${file.name}: ${serializeApiError(error).message}`)
        }
      }
      if (failures.length) onError(failures.join('; '))
    } finally {
      setBusy(false)
    }
  }

  async function open(id: string) {
    onError(null)
    try { await openFile(id, token) } catch (error) { onError(serializeApiError(error).message) }
  }

  function startSelection(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || event.pointerType === 'touch' || busy || (event.target as HTMLElement).closest('button')) return
    event.preventDefault()
    root.current?.focus()
    setMenu(null)
    const base = event.metaKey || event.ctrlKey || event.shiftKey ? selected : []
    drag.current = { x: event.clientX, y: event.clientY, base }
    setSelected(base)
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function moveSelection(event: PointerEvent<HTMLDivElement>) {
    if (!drag.current) return
    const { x, y, base } = drag.current
    if (Math.hypot(event.clientX - x, event.clientY - y) < 4) return
    const rect = { left: Math.min(x, event.clientX), top: Math.min(y, event.clientY), right: Math.max(x, event.clientX), bottom: Math.max(y, event.clientY) }
    setMarquee({ left: rect.left, top: rect.top, width: rect.right - rect.left, height: rect.bottom - rect.top })
    const hits = [...(root.current?.querySelectorAll<HTMLElement>('[data-file-id]') ?? [])]
      .filter((element) => intersects(rect, element.getBoundingClientRect()))
      .map((element) => element.dataset.fileId!)
    setSelected([...new Set([...base, ...hits])])
  }

  function endSelection() {
    drag.current = null
    setMarquee(null)
  }

  function keyboard(event: KeyboardEvent<HTMLDivElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'a') {
      event.preventDefault()
      setSelected(ids)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      if (menu) closeMenu()
      else { setSelected([]); endSelection() }
    } else if ((event.key === 'Delete' || event.key === 'Backspace') && selectedFiles.length) {
      event.preventDefault()
      void runAction('delete')
    } else if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key) && !busy) {
      event.preventDefault()
      const buttons = [...(root.current?.querySelectorAll<HTMLButtonElement>('[data-file-id]') ?? [])]
      if (!buttons.length) return
      const focused = buttons.indexOf(document.activeElement as HTMLButtonElement)
      const columns = viewMode === 'grid' ? getComputedStyle(root.current!.querySelector('ul')!).gridTemplateColumns.split(' ').length : 1
      const step = event.key === 'ArrowUp' ? -columns : event.key === 'ArrowDown' ? columns : event.key === 'ArrowLeft' ? -1 : 1
      const index = focused < 0 ? 0 : Math.max(0, Math.min(buttons.length - 1, focused + step))
      const id = ids[index]
      if (event.shiftKey && !anchor.current && focused >= 0) anchor.current = ids[focused]
      setSelected((current) => selectFileIds(current, ids, id, anchor.current, { toggle: false, range: event.shiftKey }))
      if (!event.shiftKey) anchor.current = id
      buttons[index].focus()
    }
  }

  if (files.length === 0) return null

  return (
    <section className="file-browser" aria-label="Обзор файлов" aria-busy={busy}>
      <div className="file-selection-toolbar">
        <span role="status" aria-live="polite">{busy ? 'Выполняем действие…' : selectedFiles.length ? `Выбрано: ${selectedFiles.length}` : 'Выберите файлы кликом или рамкой'}</span>
        <div className="file-selection-actions">
          <button type="button" className="p-button p-button-secondary" disabled={busy || !selectedFiles.length} onClick={() => void runAction('download')}>Скачать</button>
          <button type="button" className="p-button p-button-destructive" disabled={busy || !selectedFiles.length} onClick={() => void runAction('delete')}>Удалить</button>
        </div>
      </div>
      <p className="visually-hidden" id="file-selection-help">Клик — выделить, Cmd или Ctrl и клик — добавить к выделению, Shift и клик — диапазон. Двойной клик или Enter — открыть. Cmd или Ctrl и A — выбрать всё, Escape — снять выделение. Контекстное меню: правая кнопка мыши или Shift и F10.</p>
      <div ref={root} className="file-browser-content" tabIndex={0} aria-label="Выбор файлов" aria-describedby="file-selection-help" onKeyDown={keyboard}
        onPointerDown={startSelection} onPointerMove={moveSelection} onPointerUp={endSelection} onPointerCancel={endSelection} onLostPointerCapture={endSelection}>
        <ul className={`files-list${viewMode === 'grid' ? ' files-list--grid' : ''}`}>
          {files.map((file) => {
            const presentation = describeFile(file.type, file.name)
            return <li key={file.id}>
              <button className={`file-open${selected.includes(file.id) ? ' file-open--selected' : ''}`} data-file-id={file.id} type="button" aria-pressed={selected.includes(file.id)} disabled={busy}
                aria-label={file.name} title={file.name}
                onClick={(event) => {
                  setSelected((current) => selectFileIds(current, ids, file.id, anchor.current, { toggle: event.metaKey || event.ctrlKey, range: event.shiftKey }))
                  if (!event.shiftKey) anchor.current = file.id
                }}
                onDoubleClick={() => { if (!busy) void open(file.id) }}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') { event.preventDefault(); void open(file.id) }
                  if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
                    event.preventDefault()
                    const rect = event.currentTarget.getBoundingClientRect()
                    showMenu(file.id, event.currentTarget, rect.left, rect.bottom)
                  }
                }}
                onContextMenu={(event) => {
                  event.preventDefault()
                  showMenu(file.id, event.currentTarget, event.clientX, event.clientY)
                }}>
                <span className={`file-icon file-icon--${presentation.icon}`}><Icon name={presentation.icon} /></span>
                <span className="file-details"><strong>{file.name}</strong>{viewMode === 'list' && <small>{presentation.label}</small>}</span>
                {viewMode === 'list' && <span className="file-size">{formatBytes(file.size)}</span>}
              </button>
            </li>
          })}
        </ul>
      </div>
      {marquee && createPortal(<div className="file-selection-rect" style={marquee} aria-hidden="true" />, document.body)}
      {menu && createPortal(<div ref={menuRef} className="file-context-menu" role="menu" aria-label="Действия с файлами" style={{ left: menu.x, top: menu.y }} onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setMenu(null)
      }} onKeyDown={(event) => {
        const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button')]
        const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
        if (event.key === 'Escape') { event.preventDefault(); closeMenu() }
        else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
          event.preventDefault()
          const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
          buttons[next]?.focus()
        }
      }}>
        <button type="button" role="menuitem" onClick={() => void runAction('download')}>Скачать{selectedFiles.length > 1 ? ` (${selectedFiles.length})` : ''}</button>
        <button type="button" role="menuitem" className="file-context-delete" onClick={() => void runAction('delete')}>Удалить{selectedFiles.length > 1 ? ` (${selectedFiles.length})` : ''}</button>
      </div>, document.body)}
    </section>
  )
}
