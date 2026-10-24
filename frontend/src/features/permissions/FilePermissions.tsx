import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { serializeApiError } from '../../api/client'
import Icon from '../../components/Icon'
import type { SurfaceOrigin } from '../../components/surfaceAnimation'
import { useSurfaceTransition } from '../../components/useSurfaceTransition'
import StatusMessage from '../auth/StatusMessage'
import type { StoredFile } from '../files/filesSlice'
import { loadFilePermissions, revokeFilePermissions, saveFilePermissions } from './permissionsApi'
import { permissionsReceived, userPermissionsReplaced, type FilePermission, type PermissionCode } from './permissionsSlice'

const emptyPermissions: FilePermission[] = []
const choices: { code: PermissionCode; label: string }[] = [
  { code: 'files:read', label: 'Чтение и скачивание' },
  { code: 'files:write', label: 'Изменение' },
  { code: 'files:delete', label: 'Удаление' },
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
  const dialog = useRef<HTMLDialogElement>(null)
  const userInput = useRef<HTMLInputElement>(null)
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [userId, setUserId] = useState('')
  const [codes, setCodes] = useState<PermissionCode[]>(['files:read'])
  const users = [...new Set(permissions.map((permission) => permission.user_id))]

  useSurfaceTransition(dialog, open, origin, () => dialog.current?.close())

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

  function editUser(id: string) {
    setUserId(id)
    setCodes(permissions.filter((permission) => permission.user_id === id).map((permission) => permission.code))
    setMessage(null)
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
      dispatch(userPermissionsReplaced({ fileId: file.id, userId: id, codes: next }))
      setMessage(next.length ? 'Права доступа сохранены.' : 'Доступ отозван.')
      if (revoke && userId === id) setCodes([])
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
    <dialog ref={dialog} className="permissions-dialog" data-closing={!open} aria-labelledby="permissions-title" aria-describedby="permissions-file" onClose={onClose} onCancel={(event) => {
      event.preventDefault()
      if (!busy) setOpen(false)
    }}>
      <header className="permissions-dialog-header">
        <div><h2 id="permissions-title">Права доступа</h2><p id="permissions-file">{file.name}</p></div>
        <button className="p-button p-button-secondary p-button-icon-only" type="button" aria-label="Закрыть окно общего доступа" disabled={busy || !open} onClick={() => setOpen(false)}><Icon name="close" /></button>
      </header>
      {children}
      <h3 className="share-permissions-title">Права доступа</h3>
      <p className="field-hint">Владелец имеет полный доступ. Другим пользователям можно выдать отдельные права.</p>
      {loading && <p role="status">Загружаем права доступа…</p>}
      {error && <StatusMessage error={{ message: error }} />}
      {!loading && !loaded && <button className="p-button p-button-secondary" type="button" onClick={() => setAttempt((current) => current + 1)}>Повторить загрузку</button>}
      {message && <StatusMessage message={message} />}
      {loaded && !error && users.length === 0 && <p className="field-hint">Доступ есть только у владельца.</p>}
      {loaded && <ul className="permissions-users" aria-label="Пользователи с доступом">
        {users.map((id) => <li key={id}>
          <div><strong>{id}</strong><p>{choices.filter((choice) => permissions.some((permission) => permission.user_id === id && permission.code === choice.code)).map((choice) => choice.label).join(', ')}</p></div>
          <div className="permissions-user-actions">
            <button className="p-button p-button-secondary" type="button" disabled={busy} aria-label={`Изменить права ${id}`} onClick={() => editUser(id)}>Изменить</button>
            <button className="p-button p-button-destructive" type="button" disabled={busy} aria-label={`Отозвать доступ ${id}`} onClick={() => void update(id, [], true)}>Отозвать</button>
          </div>
        </li>)}
      </ul>}
      <form onSubmit={submit} aria-busy={busy}>
        <fieldset className="permissions-editor" disabled={!loaded || loading || busy}>
          <legend>Доступ пользователя</legend>
          <div className="form-field">
            <label className="form-label" htmlFor="permissions-user">UUID пользователя</label>
            <input ref={userInput} className="p-field" id="permissions-user" value={userId} onChange={(event) => {
              const id = event.target.value
              setUserId(id)
              setCodes(permissions.filter((permission) => permission.user_id === id.trim().toLowerCase()).map((permission) => permission.code))
            }} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" autoComplete="off" autoCapitalize="none" spellCheck={false} pattern="[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}" required />
          </div>
          <div className="permissions-choices">
            {choices.map(({ code, label }) => <label key={code}><input className="p-checkbox" type="checkbox" checked={codes.includes(code)} onChange={(event) => setCodes((current) => event.target.checked ? [...current, code] : current.filter((item) => item !== code))} />{label}</label>)}
          </div>
          <p className="field-hint">Сохранение заменит права этого пользователя. Если снять все отметки, доступ будет отозван.</p>
          <div className="form-actions"><button className="p-button p-button-primary" type="submit">{busy ? 'Сохраняем…' : 'Сохранить права'}</button></div>
        </fieldset>
      </form>
    </dialog>, document.body,
  )
}
