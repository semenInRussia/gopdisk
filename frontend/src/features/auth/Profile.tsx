import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
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

  useEffect(() => {
    if (!userInfoOpen) return

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

  if (!user) return <main className="auth-card loading-card">Загружаем профиль…</main>

  return (
    <div className="profile-page">
      <main className="auth-card profile-card">
        <header className="profile-header">
          <div>
            <p className="eyebrow">Личное хранилище</p>
            <h1>GopherDisk</h1>
          </div>
        </header>
        <FileUpload ownerId={user.id} />
      </main>
      <div className="account-menu" ref={accountRef}>
        <button
          className="floating-button account-button"
          type="button"
          ref={accountButtonRef}
          aria-label="Информация о пользователе"
          aria-expanded={userInfoOpen}
          aria-controls="user-info"
          onClick={() => setUserInfoOpen((open) => !open)}
        >
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="9" cy="8" r="3" />
            <path d="M3 20v-2a6 6 0 0 1 12 0v2M16 5a3 3 0 0 1 0 6M21 20v-2a6 6 0 0 0-4-5.65" />
          </svg>
        </button>
        {userInfoOpen && (
          <section className="account-panel" id="user-info" aria-labelledby="user-info-heading">
            <h2 id="user-info-heading">Моя информация</h2>
            <span className={`account-status ${user.activated ? 'account-status--active' : ''}`}>
              {user.activated ? 'Активирован' : 'Не активирован'}
            </span>
            <dl className="profile-list">
              <div><dt>ID пользователя</dt><dd>{user.id}</dd></div>
              <div><dt>Имя</dt><dd>{user.name}</dd></div>
              <div><dt>Электронная почта</dt><dd>{user.email}</dd></div>
            </dl>
            <button className="secondary-button" type="button" onClick={handleSignOut}>Выйти</button>
          </section>
        )}
      </div>
    </div>
  )
}
