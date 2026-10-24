import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import Icon from '../../components/Icon'
import FileUpload from '../files/FileUpload'
import { filesCleared } from '../files/filesSlice'
import { currentUserSet, selectCurrentUser } from '../users/usersSlice'
import { signedOut } from './authSlice'

export default function Profile() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const user = useAppSelector(selectCurrentUser)
  const [userInfoOpen, setUserInfoOpen] = useState(false)
  const accountRef = useRef<HTMLDivElement>(null)
  const accountButtonRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLElement>(null)

  useEffect(() => {
    if (!userInfoOpen) return
    panelRef.current?.focus()

    function handleOutsideClick(event: PointerEvent) {
      if (event.target instanceof Node && !accountRef.current?.contains(event.target)) {
        setUserInfoOpen(false)
      }
    }

    function handleEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setUserInfoOpen(false)
        accountButtonRef.current?.focus()
      }
    }

    document.addEventListener('pointerdown', handleOutsideClick)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('pointerdown', handleOutsideClick)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [userInfoOpen])

  function handleSignOut() {
    dispatch(signedOut())
    dispatch(currentUserSet(null))
    dispatch(filesCleared())
    navigate('/login', { replace: true })
  }

  if (!user) return <main className="auth-card loading-card" role="status"><span className="p-spinner p-spinner-small" aria-hidden="true" />Загружаем профиль…</main>

  return (
    <div className="profile-page">
      <main className="auth-card profile-card">
        <header className="profile-header">
          <div>
            <p className="eyebrow"><Icon name="box" /> GopherDisk</p>
            <h1>Мои файлы</h1>
            <p className="profile-subtitle">Всё важное — в одном месте.</p>
          </div>
        </header>
        <FileUpload ownerId={user.id} />
      </main>
      <div className="account-menu" ref={accountRef} onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setUserInfoOpen(false)
      }}>
        <button
          className="p-button p-button-secondary p-button-icon-only floating-button account-button"
          type="button"
          ref={accountButtonRef}
          aria-label="Информация о пользователе"
          title="Информация о пользователе"
          aria-haspopup="dialog"
          aria-expanded={userInfoOpen}
          aria-controls="user-info"
          onClick={() => setUserInfoOpen((open) => !open)}
        >
          <Icon name="person" />
        </button>
        {userInfoOpen && (
          <section className="p-material-thick account-panel" id="user-info" role="dialog" aria-labelledby="user-info-heading" tabIndex={-1} ref={panelRef}>
            <div className="account-panel-heading">
              <h2 id="user-info-heading">Моя информация</h2>
              <button className="p-button p-button-icon-only p-button-touch panel-close" type="button" aria-label="Закрыть информацию о пользователе" onClick={() => {
                setUserInfoOpen(false)
                accountButtonRef.current?.focus()
              }}><Icon name="close" /></button>
            </div>
            <span className={`account-status ${user.activated ? 'account-status--active' : ''}`}>
              <Icon name={user.activated ? 'check' : 'warning'} />
              {user.activated ? 'Активирован' : 'Не активирован'}
            </span>
            <dl className="profile-list">
              <div><dt>ID пользователя</dt><dd>{user.id}</dd></div>
              <div><dt>Имя</dt><dd>{user.name}</dd></div>
              <div><dt>Электронная почта</dt><dd>{user.email}</dd></div>
            </dl>
            <button className="p-button p-button-secondary p-button-touch-large" type="button" onClick={handleSignOut}>Выйти</button>
          </section>
        )}
      </div>
    </div>
  )
}
