import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { serializeApiError } from '../../api/client'
import Icon from '../../components/Icon'
import type { SurfaceOrigin } from '../../components/surfaceAnimation'
import { useSurfaceTransition } from '../../components/useSurfaceTransition'
import { useDialogViewport } from '../../components/useDialogViewport'
import StatusMessage from '../auth/StatusMessage'
import type { StoredFile } from '../files/filesSlice'
import { selectCurrentUser } from '../users/usersSlice'
import { loadFilePermissions, revokeFilePermissions, saveFilePermissions } from './permissionsApi'
import { permissionsReceived, userPermissionsReplaced, type FilePermission, type PermissionCode } from './permissionsSlice'

const emptyPermissions: FilePermission[] = []
const choices: { code: PermissionCode; label: string; description: string }[] = [
  { code: 'files:read', label: 'Чтение и скачивание', description: 'Открывать файл и сохранять его на устройство' },
  { code: 'files:write', label: 'Изменение', description: 'Обновлять содержимое файла' },
  { code: 'files:delete', label: 'Удаление', description: 'Удалять файл из хранилища' },
]

interface Props {
  file: StoredFile
  token: string | null
  onClose: () => void
  children?: ReactNode
  origin: SurfaceOrigin
}

export default function FilePermissions({ file, token, origin, onClose, children }: Props) {
  const dispatch = useAppDispatch()
  const permissions = useAppSelector((state) => state.permissions.byFileId[file.id] ?? emptyPermissions)
  const currentUser = useAppSelector(selectCurrentUser)
  const knownUsers = useAppSelector((state) => state.users.entities)
  const dialog = useRef<HTMLDialogElement>(null)
  const userInput = useRef<HTMLInputElement>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  const restoreEditorFocus = useRef(false)
  const keepEditingButton = useRef<HTMLButtonElement>(null)
  const [discardRequested, setDiscardRequested] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [userId, setUserId] = useState('')
  const [codes, setCodes] = useState<PermissionCode[]>(['files:read'])
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [addedUser, setAddedUser] = useState<string | null>(null)
  const users = [...new Set(permissions.map((permission) => permission.user_id))].filter((id) => id !== file.owner_id)
  const originalCodes = editingId ? permissions.filter((permission) => permission.user_id === editingId).map((permission) => permission.code) : ['files:read']
  const dirty = editorOpen && ((!editingId && userId.trim() !== '') || codes.length !== originalCodes.length || codes.some((code) => !originalCodes.includes(code)))

  function requestClose() {
    if (busy) return
    if (discardRequested) { setDiscardRequested(false); return }
    if (dirty) setDiscardRequested(true)
    else setOpen(false)
  }

  useEffect(() => {
    if (discardRequested) keepEditingButton.current?.focus()
    else if (editorOpen) userInput.current?.focus()
  }, [discardRequested])

  useSurfaceTransition(dialog, open, origin, () => dialog.current?.close(), true)
  useDialogViewport(dialog)

  useEffect(() => {
    dialog.current?.querySelector<HTMLButtonElement>('.permissions-close-button')?.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setLoaded(false)
    setError(null)
    loadFilePermissions(file.id, token, controller.signal).then((result) => {
      if (!controller.signal.aborted) {
        dispatch(permissionsReceived({ fileId: file.id, permissions: result }))
        setLoaded(true)
      }
    }).catch((failure: unknown) => {
      if (!controller.signal.aborted) setError(serializeApiError(failure).message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [dispatch, file.id, token, attempt])

  useEffect(() => {
    if (editorOpen) userInput.current?.focus()
    else if (!busy && restoreEditorFocus.current) {
      addButton.current?.focus()
      restoreEditorFocus.current = false
    }
  }, [editorOpen, editingId, busy])

  function closeEditor() {
    restoreEditorFocus.current = true
    setError(null)
    setEditorOpen(false)
    setEditingId(null)
    setUserId('')
    setCodes(['files:read'])
    addButton.current?.focus()
  }

  function editUser(id: string) {
    setEditingId(id)
    setEditorOpen(true)
    setUserId(id)
    setCodes(permissions.filter((permission) => permission.user_id === id).map((permission) => permission.code))
    setMessage(null)
    setError(null)
    userInput.current?.focus()
  }

  async function update(id: string, next: PermissionCode[], revoke = false) {
    setError(null)
    setMessage(null)
    if (id === file.owner_id) {
      setError('Владелец всегда имеет полный доступ к файлу.')
      return
    }
    setBusy(true)
    try {
      if (revoke) await revokeFilePermissions(file.id, id, token)
      else await saveFilePermissions(file.id, id, next, token)
      if (next.length && !users.includes(id)) setAddedUser(id)
      dispatch(userPermissionsReplaced({ fileId: file.id, userId: id, codes: next }))
      setMessage(next.length ? 'Права доступа сохранены.' : 'Доступ отозван.')
      if (!revoke || userId.trim().toLowerCase() === id) closeEditor()
    } catch (failure) {
      setError(serializeApiError(failure).message)
    } finally {
      setBusy(false)
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!busy && loaded && !loading) void update(userId.trim().toLowerCase(), codes)
  }

  return createPortal(
    <dialog ref={dialog} className="permissions-dialog" data-closing={!open} aria-modal="true" aria-labelledby="permissions-title" aria-describedby="permissions-file" onClose={onClose} onCancel={(event) => {
      event.preventDefault()
      requestClose()
    }}>
      <header className="permissions-dialog-header">
        <div><h2 id="permissions-title">{file.name}</h2><p id="permissions-file">Права доступа</p></div>
        <button className="p-button p-button-secondary p-button-icon-only permissions-close-button" type="button" aria-label="Закрыть окно общего доступа" autoFocus disabled={busy || !open || discardRequested} onClick={requestClose}><Icon name="close" /></button>
      </header>
      {discardRequested && <section className="permissions-discard" aria-labelledby="permissions-discard-title">
        <div role="alert"><h3 id="permissions-discard-title">Отменить изменения?</h3><p>Новые права ещё не сохранены. При закрытии окна изменения будут потеряны.</p></div>
        <div className="permissions-editor-actions">
          <button ref={keepEditingButton} type="button" className="p-button p-button-secondary" onClick={() => setDiscardRequested(false)}>Продолжить редактирование</button>
          <button type="button" className="p-button p-button-destructive" onClick={() => setOpen(false)}>Отменить изменения</button>
        </div>
      </section>}
      <div className="permissions-dialog-body" inert={discardRequested}>
      {children}
      <section className="permissions-access-section" aria-labelledby="permissions-access-title">
      <div className="permissions-section-heading">
        <div className="share-section-heading"><Icon name="people" /><h3 id="permissions-access-title">У кого есть доступ</h3>{loaded && <span className="file-count" aria-label={`Пользователей с доступом: ${users.length + 1}`}>{users.length + 1}</span>}</div>
        <button ref={addButton} className="p-button p-button-borderless p-button-icon-only permissions-add-button" type="button" aria-label="Добавить пользователя" title="Добавить пользователя" disabled={!loaded || loading || busy || editorOpen} aria-expanded={editorOpen} aria-controls="permissions-editor-form" onClick={() => {
          setEditingId(null)
          setUserId('')
          setCodes(['files:read'])
          setMessage(null)
          setError(null)
          setEditorOpen(true)
        }}><Icon name="plus" /></button>
      </div>
      {loading && <p className="permissions-loading" role="status"><span className="p-spinner p-spinner-small" aria-hidden="true" />Загружаем права доступа…</p>}
      {error && !editorOpen && <StatusMessage error={{ message: error }} />}
      {!loading && !loaded && <button className="p-button p-button-secondary" type="button" onClick={() => setAttempt((current) => current + 1)}>Повторить загрузку</button>}
      {message && <StatusMessage message={message} />}
      <ul className="p-list p-list-inset permissions-users" aria-label="Пользователи с доступом">
        <li className="permissions-owner">
          <span className="permissions-avatar"><Icon name="person" /></span>
          <div className="permissions-user-info"><strong>{knownUsers[file.owner_id]?.name ?? 'Владелец файла'}{currentUser?.id === file.owner_id && ' (вы)'}</strong><p>Полный доступ</p></div>
          <span className="permissions-owner-badge">Владелец</span>
        </li>
        {loaded && users.map((id) => <li key={id} className={addedUser === id ? 'permissions-user--added' : undefined} onAnimationEnd={(event) => {
          if (event.target === event.currentTarget && event.animationName === 'permission-user-enter') setAddedUser(null)
        }}>
          <span className="permissions-avatar permissions-avatar--guest"><Icon name="person" /></span>
          <div className="permissions-user-info"><strong>{knownUsers[id]?.name ?? id}</strong>{knownUsers[id] && <small>{id}</small>}<p>{choices.filter((choice) => permissions.some((permission) => permission.user_id === id && permission.code === choice.code)).map((choice) => choice.label).join(' · ')}</p></div>
          <div className="permissions-user-actions">
            <button className="p-button p-button-secondary" type="button" disabled={busy} aria-label={`Изменить права ${id}`} onClick={() => editUser(id)}>Изменить</button>
            <button className="p-button p-button-destructive" type="button" disabled={busy} aria-label={`Отозвать доступ ${id}`} onClick={() => void update(id, [], true)}>Отозвать</button>
          </div>
        </li>)}
      </ul>
      {loaded && users.length === 0 && <p className="field-hint permissions-empty">Пока только вы. Добавьте пользователя, чтобы открыть ему доступ по ссылке.</p>}
      </section>
      {editorOpen && <form id="permissions-editor-form" className="permissions-editor-form" onSubmit={submit} aria-busy={busy}>
        <fieldset className="permissions-editor" disabled={!loaded || loading || busy}>
          <legend>{editingId ? 'Изменить права' : 'Добавить пользователя'}</legend>
          <div className="form-field">
            <label className="form-label" htmlFor="permissions-user">ID пользователя</label>
            <input ref={userInput} className="p-field" id="permissions-user" value={userId} readOnly={editingId !== null} aria-describedby="permissions-user-hint" onChange={(event) => {
              const id = event.target.value
              setUserId(id)
              const existing = permissions.filter((permission) => permission.user_id === id.trim().toLowerCase()).map((permission) => permission.code)
              setCodes(existing.length ? existing : ['files:read'])
            }} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" autoComplete="off" autoCapitalize="none" spellCheck={false} pattern="[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}" required />
            <small id="permissions-user-hint" className="field-hint">UUID из раздела «Моя информация» в профиле получателя.</small>
          </div>
          <div className="p-list p-list-inset permissions-choices">
            {choices.map(({ code, label, description }) => <label key={code}><input className="p-checkbox" type="checkbox" checked={codes.includes(code)} onChange={(event) => setCodes((current) => event.target.checked ? [...current, code] : current.filter((item) => item !== code))} /><span><strong>{label}</strong><small>{description}</small></span></label>)}
          </div>
          <p className={`field-hint${codes.length === 0 ? ' permissions-revoke-hint' : ''}`}>{codes.length === 0 ? 'Все права сняты. При сохранении доступ пользователя будет отозван.' : 'Пользователь получит только выбранные права.'}</p>
          {error && <StatusMessage error={{ message: error }} />}
          <div className="permissions-editor-actions"><button className="p-button p-button-secondary" type="button" onClick={closeEditor}>Отмена</button><button className={`p-button ${codes.length ? 'p-button-primary' : 'p-button-destructive'}`} type="submit" disabled={!userId.trim()}>{busy ? 'Сохраняем…' : codes.length ? 'Сохранить права' : 'Отозвать доступ'}</button></div>
        </fieldset>
      </form>}
      </div>
    </dialog>, document.body,
  )
}
