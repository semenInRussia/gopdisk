import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type MouseEvent, type ReactNode, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { serializeApiError } from '../../api/client'
import Icon from '../../components/Icon'
import ToolbarGroup from '../../components/ToolbarGroup'
import { activationPoint, type SurfaceOrigin } from '../../components/surfaceAnimation'
import { fileRemoved, type FileViewMode, type StoredFile } from './filesSlice'
import { describeFile, formatBytes } from './filePresentation'
import { deleteStoredFile, downloadStoredFiles } from './fileActions'
import { intersects, selectFileIds } from './fileSelection'
import { openFile } from './openFile'
import { animateFileDeletion } from './fileDeletionAnimation'
import { selectCurrentUser } from '../users/usersSlice'
import ShareFileDialog from './ShareFileDialog'
import DeleteConfirmation, { type DeleteAnchor } from './DeleteConfirmation'

interface Props {
  files: StoredFile[]
  viewMode: FileViewMode
  token: string | null
  onError: (message: string | null) => void
  toolbarControls?: ReactNode
  totalFiles?: number
}

export default function FileBrowser({ files, viewMode, token, onError, toolbarControls, totalFiles = files.length }: Props) {
  const dispatch = useAppDispatch()
  const currentUser = useAppSelector(selectCurrentUser)
  const [selected, setSelected] = useState<string[]>([])
  const [sharedFile, setSharedFile] = useState<StoredFile | null>(null)
  const [shareOrigin, setShareOrigin] = useState<SurfaceOrigin>({ x: 0, y: 0 })
  const [busy, setBusy] = useState(false)
  const [deleteTargets, setDeleteTargets] = useState<StoredFile[] | null>(null)
  const [deleteAnchor, setDeleteAnchor] = useState<DeleteAnchor | null>(null)
  const [downloadProgress, setDownloadProgress] = useState<string | null>(null)
  const [toolbarStuck, setToolbarStuck] = useState(false)
  const [toolbarFloating, setToolbarFloating] = useState(false)
  const [toolbarHeight, setToolbarHeight] = useState(0)
  const [menu, setMenu] = useState<{ x: number; y: number; touch: boolean } | null>(null)
  const [marquee, setMarquee] = useState<{ left: number; top: number; width: number; height: number } | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const toolbarSentinel = useRef<HTMLDivElement>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)
  const toolbarSlot = useRef<HTMLDivElement>(null)
  const toolbarFrame = useRef<Keyframe | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const anchor = useRef<string | null>(null)
  const drag = useRef<{ x: number; y: number; base: string[] } | null>(null)
  const touchSelection = useRef(false)
  const touchHold = useRef<{ timer: number; pointerId: number; x: number; y: number } | null>(null)
  const suppressTouchClick = useRef(false)
  const ids = files.map((file) => file.id)
  const selectedFiles = files.filter((file) => selected.includes(file.id))
  const hasFiles = files.length > 0
  const hasToolbar = hasFiles || Boolean(toolbarControls)

  function cancelTouchHold() {
    if (touchHold.current) window.clearTimeout(touchHold.current.timer)
    touchHold.current = null
  }

  useEffect(() => {
    const cancel = () => {
      cancelTouchHold()
      touchSelection.current = false
    }
    const anotherPointer = (event: globalThis.PointerEvent) => {
      if (touchHold.current && event.pointerId !== touchHold.current.pointerId) cancel()
    }
    window.addEventListener('scroll', cancel, true)
    window.addEventListener('blur', cancel)
    window.addEventListener('pointerdown', anotherPointer)
    return () => {
      cancel()
      window.removeEventListener('scroll', cancel, true)
      window.removeEventListener('blur', cancel)
      window.removeEventListener('pointerdown', anotherPointer)
    }
  }, [busy, files])

  function startTouchHold(event: PointerEvent<HTMLButtonElement>, id: string) {
    cancelTouchHold()
    suppressTouchClick.current = false
    touchSelection.current = event.pointerType === 'touch'
    if (!touchSelection.current || event.isPrimary === false || busy) return
    const element = event.currentTarget
    const { clientX: x, clientY: y, pointerId } = event
    const timer = window.setTimeout(() => {
      touchHold.current = null
      suppressTouchClick.current = true
      showMenu(id, element, x, y, true)
    }, 500)
    touchHold.current = { timer, pointerId, x, y }
  }

  useEffect(() => {
    const sentinel = toolbarSentinel.current
    const toolbar = toolbarRef.current
    if (!sentinel || !toolbar) return
    const measure = () => setToolbarHeight(toolbar.getBoundingClientRect().height)
    measure()
    const resizeObserver = new ResizeObserver(measure)
    resizeObserver.observe(toolbar)
    const observer = new IntersectionObserver(([entry]) => {
      setToolbarStuck(!entry.isIntersecting && entry.boundingClientRect.top < 0)
    })
    observer.observe(sentinel)
    return () => {
      observer.disconnect()
      resizeObserver.disconnect()
    }
  }, [hasToolbar])

  useLayoutEffect(() => {
    const toolbar = toolbarRef.current
    const slot = toolbarSlot.current
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (!hasToolbar) { setToolbarFloating(false); return }
    if (toolbarStuck && !toolbarFloating) { setToolbarFloating(true); return }
    if (!toolbarFloating || !toolbar || !slot) return
    if (reducedMotion.matches) {
      toolbarFrame.current = null
      setToolbarFloating(toolbarStuck)
      return
    }

    const appearance = getComputedStyle(toolbar)
    const fullFrame: Keyframe = {
      left: '0px', right: '0px', top: '0px',
      paddingLeft: appearance.paddingLeft,
      paddingRight: appearance.paddingRight,
      borderRadius: '0px',
      boxShadow: appearance.boxShadow,
    }
    function compactFrame(): Keyframe {
      const origin = slot!.getBoundingClientRect()
      return {
        left: `${origin.left}px`,
        right: `${Math.max(0, document.documentElement.clientWidth - origin.right)}px`,
        top: `${Math.max(0, origin.top)}px`,
        paddingLeft: '0px',
        paddingRight: '0px',
        borderRadius: '0px',
        boxShadow: 'none',
      }
    }
    const from = toolbarFrame.current ?? (toolbarStuck ? { ...compactFrame(), borderRadius: '0 0 16px 16px' } : fullFrame)
    toolbarFrame.current = null
    const animation = toolbar.animate([from, toolbarStuck ? fullFrame : compactFrame()], {
      duration: toolbarStuck ? 460 : 360,
      easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
      fill: 'both',
    })
    animation.onfinish = () => {
      if (toolbarStuck) animation.cancel()
      else setToolbarFloating(false)
    }

    // Keep the return destination attached to the page as scrolling continues.
    const followScroll = () => {
      if (!toolbarStuck && animation.playState === 'running') {
        (animation.effect as KeyframeEffect).setKeyframes([from, compactFrame()])
      }
    }
    const finish = () => animation.finish()
    reducedMotion.addEventListener('change', finish)
    window.addEventListener('resize', finish)
    window.addEventListener('scroll', followScroll, { passive: true })
    return () => {
      if (animation.playState === 'running') {
        const rect = toolbar.getBoundingClientRect()
        const style = getComputedStyle(toolbar)
        toolbarFrame.current = {
          left: `${rect.left}px`, right: `${document.documentElement.clientWidth - rect.right}px`, top: `${rect.top}px`,
          paddingLeft: style.paddingLeft, paddingRight: style.paddingRight,
          borderRadius: style.borderRadius, boxShadow: style.boxShadow,
        }
      }
      animation.onfinish = null
      animation.cancel()
      reducedMotion.removeEventListener('change', finish)
      window.removeEventListener('resize', finish)
      window.removeEventListener('scroll', followScroll)
    }
  }, [toolbarStuck, toolbarFloating, hasToolbar])

  useEffect(() => {
    setSelected((current) => {
      const remaining = current.filter((id) => files.some((file) => file.id === id))
      return remaining.length === current.length ? current : remaining
    })
  }, [files])

  function closeMenu() {
    setMenu(null)
    if (returnFocus.current?.isConnected) returnFocus.current.focus({ preventScroll: true })
    root.current?.focus({ preventScroll: true })
  }

  function shareFile(event: MouseEvent<HTMLButtonElement>) {
    if (busy || selectedFiles.length !== 1) return
    const file = selectedFiles[0]
    setShareOrigin(activationPoint(event))
    setMenu(null)
    setSharedFile(file)
  }

  function showMenu(id: string, element: HTMLButtonElement, x: number, y: number, touch = false) {
    if (busy) return
    if (!selected.includes(id)) { setSelected([id]); anchor.current = id }
    returnFocus.current = element
    setMenu({ x, y, touch })
  }

  useLayoutEffect(() => {
    const element = menuRef.current
    if (!menu || !element) return
    const { width, height } = element.getBoundingClientRect()
    const viewport = window.visualViewport
    const leftEdge = (viewport?.offsetLeft ?? 0) + 8
    const topEdge = (viewport?.offsetTop ?? 0) + 8
    const rightEdge = leftEdge + (viewport?.width ?? window.innerWidth) - 16
    const bottomEdge = topEdge + (viewport?.height ?? window.innerHeight) - 16
    const anchor = returnFocus.current?.getBoundingClientRect()
    let x = menu.x
    let y = menu.y
    if (menu.touch && anchor) {
      x = anchor.left + width <= rightEdge ? anchor.left : anchor.right - width
      y = anchor.bottom + 16
      if (y + height > bottomEdge) {
        y = anchor.top
        if (anchor.right + 16 + width <= rightEdge) x = anchor.right + 16
        else if (anchor.left - 16 - width >= leftEdge) x = anchor.left - 16 - width
        else y = anchor.top - 16 - height
      }
    }
    element.style.left = `${Math.max(leftEdge, Math.min(x, rightEdge - width))}px`
    element.style.top = `${Math.max(topEdge, Math.min(y, bottomEdge - height))}px`
  }, [menu])

  useEffect(() => {
    if (!menu) return
    menuRef.current?.querySelector<HTMLButtonElement>('button')?.focus({ preventScroll: true })
    function outside(event: globalThis.PointerEvent) {
      if (event.target instanceof Node && !menuRef.current?.contains(event.target)) setMenu(null)
    }
    const dismiss = () => setMenu(null)
    // The compatibility click from lifting the held finger may target a menu
    // command. Consume it before either React or the browser can activate it.
    const releaseClick = (event: globalThis.MouseEvent) => {
      if (!suppressTouchClick.current || event.detail === 0) return
      suppressTouchClick.current = false
      touchSelection.current = false
      event.preventDefault()
      event.stopImmediatePropagation()
    }
    const newGesture = () => { suppressTouchClick.current = false }
    document.addEventListener('click', releaseClick, true)
    document.addEventListener('pointerdown', newGesture, true)
    document.addEventListener('pointerdown', outside)
    window.addEventListener('blur', dismiss)
    window.addEventListener('resize', dismiss)
    window.addEventListener('scroll', dismiss, true)
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('click', releaseClick, true)
      document.removeEventListener('pointerdown', newGesture, true)
      window.removeEventListener('blur', dismiss)
      window.removeEventListener('resize', dismiss)
      window.removeEventListener('scroll', dismiss, true)
    }
  }, [menu])

  async function runAction(action: 'download' | 'delete', confirmedFiles?: StoredFile[], trigger?: HTMLElement) {
    const targets = [...(confirmedFiles ?? selectedFiles)]
    if (busy || targets.length === 0) return
    if (action === 'delete' && !confirmedFiles) {
      const source = trigger ?? document.activeElement as HTMLElement | null
      const tether = source?.closest('[role="menu"]') ? returnFocus.current : source
      const sourceRect = source?.getBoundingClientRect()
      const tetherRect = tether?.getBoundingClientRect()
      setDeleteAnchor(tether ? {
        element: tether,
        offset: source !== tether && sourceRect && tetherRect ? {
          left: sourceRect.left - tetherRect.left, top: sourceRect.top - tetherRect.top,
          width: sourceRect.width, height: sourceRect.height,
        } : undefined,
      } : null)
      closeMenu()
      setDeleteTargets(targets)
      return
    }
    closeMenu()
    setBusy(true)
    onError(null)
    const failures: string[] = []
    const removals: Promise<void>[] = []
    try {
      if (action === 'download') {
        await downloadStoredFiles(targets, token, (completed, total) => setDownloadProgress(`Готовим архив: ${completed} из ${total}`))
        return
      }
      for (const file of targets) {
        try {
          await deleteStoredFile(file.id, token)
          const button = [...(root.current?.querySelectorAll<HTMLElement>('[data-file-id]') ?? [])].find((element) => element.dataset.fileId === file.id)
          removals.push(animateFileDeletion(button?.closest('li') ?? null).then(() => {
            dispatch(fileRemoved(file.id))
            setSelected((current) => current.filter((id) => id !== file.id))
          }))
        } catch (error) {
          failures.push(`${file.name}: ${serializeApiError(error).message}`)
        }
      }
      if (failures.length) onError(failures.join('; '))
    } catch (error) {
      onError(serializeApiError(error).message)
    } finally {
      await Promise.all(removals)
      setBusy(false)
      setDownloadProgress(null)
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
      void runAction('delete', undefined, event.target as HTMLElement)
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

  if (!hasToolbar) return null

  return (
    <section className="file-browser" aria-label="Обзор файлов" aria-busy={busy} style={{ '--file-toolbar-height': `${toolbarHeight}px` } as CSSProperties}>
      <div className="file-toolbar-sentinel" ref={toolbarSentinel} aria-hidden="true" />
      <div ref={toolbarSlot} className="file-selection-toolbar-slot" style={{ height: toolbarHeight || undefined }}>
        <div ref={toolbarRef} className={`file-selection-toolbar${toolbarFloating ? ' p-material-thick file-selection-toolbar--stuck' : ''}`}>
          {(downloadProgress || busy) && <span role="status" aria-live="polite">{downloadProgress ?? 'Выполняем действие…'}</span>}
          <div className="file-toolbar-main">
          <ToolbarGroup label="Действия с выбранными файлами" className="file-selection-actions">
            <button type="button" className="p-toolbar-btn file-touch-open" aria-label="Открыть" title="Открыть" disabled={busy || selectedFiles.length !== 1} onClick={() => void open(selectedFiles[0].id)}><Icon name="open" /><span>Открыть</span></button>
            <button type="button" className="p-toolbar-btn" aria-label="Права доступа" title="Права доступа" disabled={busy || selectedFiles.length !== 1} onClick={(event) => void shareFile(event)}><Icon name="link" /><span>Права доступа</span></button>
            <button type="button" className="p-toolbar-btn" aria-label="Скачать" title="Скачать" disabled={busy || !selectedFiles.length} onClick={() => void runAction('download')}><Icon name="download" /><span>Скачать</span></button>
            <div className="p-toolbar-divider" role="separator" aria-orientation="vertical" />
            <button type="button" className="p-toolbar-btn file-toolbar-delete" aria-label="Удалить" title="Удалить" disabled={busy || !selectedFiles.length} onClick={(event) => void runAction('delete', undefined, event.currentTarget)}><Icon name="trash" /><span>Удалить</span></button>
          </ToolbarGroup>
          {toolbarControls}
          </div>
        </div>
      </div>
      {createPortal(<div className="p-material-thick file-page-counters" role="status" aria-live="polite" data-visible={toolbarFloating} aria-hidden={!toolbarFloating} inert={!toolbarFloating}>
        <span>Выбрано: <strong>{selectedFiles.length}</strong></span>
        <span>Всего файлов: <strong>{totalFiles}</strong></span>
      </div>, document.body)}
      <p className="visually-hidden" id="file-selection-help">На сенсорном экране касание добавляет файл к выделению или снимает выделение. Удерживайте палец на файле, чтобы открыть контекстное меню. Для открытия выберите один файл и нажмите «Открыть». Клик — выделить, Cmd или Ctrl и клик — добавить к выделению, Shift и клик — диапазон. Двойной клик или Enter — открыть. Cmd или Ctrl и A — выбрать всё, Escape — снять выделение. Контекстное меню: правая кнопка мыши или Shift и F10.</p>
      <div ref={root} className="file-browser-content" hidden={!hasFiles} tabIndex={0} aria-label="Выбор файлов" aria-describedby="file-selection-help" onKeyDown={keyboard}
        onPointerDown={startSelection} onPointerMove={moveSelection} onPointerUp={endSelection} onPointerCancel={endSelection} onLostPointerCapture={endSelection}>
        <ul className={`files-list${viewMode === 'grid' ? ' files-list--grid' : ''}`}>
          {files.map((file) => {
            const presentation = describeFile(file.type, file.name)
            return <li key={file.id}>
              <button className={`file-open${selected.includes(file.id) ? ' file-open--selected' : ''}`} data-file-id={file.id} type="button" aria-pressed={selected.includes(file.id)} disabled={busy}
                aria-label={file.name} title={file.name}
                onPointerDown={(event) => startTouchHold(event, file.id)}
                onPointerMove={(event) => {
                  const hold = touchHold.current
                  if (hold && Math.hypot(event.clientX - hold.x, event.clientY - hold.y) > 10) cancelTouchHold()
                }}
                onPointerUp={cancelTouchHold}
                onPointerCancel={() => { cancelTouchHold(); touchSelection.current = false }}
                onLostPointerCapture={cancelTouchHold}
                onClick={(event) => {
                  if (suppressTouchClick.current) {
                    event.preventDefault()
                    suppressTouchClick.current = false
                    touchSelection.current = false
                    return
                  }
                  const toggle = touchSelection.current || event.metaKey || event.ctrlKey
                  touchSelection.current = false
                  setSelected((current) => selectFileIds(current, ids, file.id, anchor.current, { toggle, range: event.shiftKey }))
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
                  cancelTouchHold()
                  if (touchSelection.current) suppressTouchClick.current = true
                  showMenu(file.id, event.currentTarget, event.clientX, event.clientY, touchSelection.current || suppressTouchClick.current)
                }}>
                <span className={`file-icon file-icon--${presentation.icon}`}><Icon name={presentation.icon} /></span>
                {viewMode === 'list' && <>
                  <span className="file-details"><strong>{file.name}</strong><small>{presentation.label}</small></span>
                  <span className="file-size">{formatBytes(file.size)}</span>
                </>}
                {viewMode === 'grid' && <span className="file-details" aria-hidden="true"><strong>{file.name}</strong></span>}
              </button>
            </li>
          })}
        </ul>
      </div>
      {marquee && createPortal(<div className="file-selection-rect" style={marquee} aria-hidden="true" />, document.body)}
      {menu && createPortal(<div ref={menuRef} className={`p-menu file-context-menu${menu.touch ? ' p-menu-touch p-menu-touch--context' : ''}`} role="menu" aria-label="Действия с файлами" style={{ left: menu.x, top: menu.y }} onMouseDown={(event) => {
        // Safari may blur to the page before click when pressing a menu button.
        if (event.button === 0) event.preventDefault()
      }} onBlur={(event) => {
        if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget)) setMenu(null)
      }} onKeyDown={(event) => {
        const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
        const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
        if (event.key === 'Escape') { event.preventDefault(); closeMenu() }
        else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
          event.preventDefault()
          const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
          buttons[next]?.focus()
        }
      }}>
        <button type="button" role="menuitem" className="p-menu__item" onClick={() => void runAction('download')}><span className="p-menu__icon"><Icon name="download" /></span>Скачать{selectedFiles.length > 1 ? ` (${selectedFiles.length})` : ''}</button>
        <button type="button" role="menuitem" className="p-menu__item" disabled={selectedFiles.length !== 1} title="Права доступа и ссылка на выбранный файл" onClick={(event) => void shareFile(event)}><span className="p-menu__icon"><Icon name="lock" /></span>Права доступа</button>
        <div className="p-menu__separator" role="separator" />
        <button type="button" role="menuitem" className="p-menu__item p-menu__item--destructive" onClick={(event) => void runAction('delete', undefined, event.currentTarget)}><span className="p-menu__icon"><Icon name="trash" /></span>Удалить{selectedFiles.length > 1 ? ` (${selectedFiles.length})` : ''}</button>
      </div>, document.body)}
      {sharedFile && <ShareFileDialog key={sharedFile.id} file={sharedFile} origin={shareOrigin} initialStatus="idle" token={token} canManagePermissions={sharedFile.owner_id === currentUser?.id} onClose={() => {
        setSharedFile(null)
        if (returnFocus.current?.isConnected) returnFocus.current.focus()
        root.current?.focus()
      }} />}
      {deleteTargets && <DeleteConfirmation files={deleteTargets} anchor={deleteAnchor} onCancel={() => {
        setDeleteTargets(null)
        root.current?.focus({ preventScroll: true })
      }} onConfirm={() => {
        const targets = deleteTargets
        setDeleteTargets(null)
        void runAction('delete', targets)
      }} />}
    </section>
  )
}
