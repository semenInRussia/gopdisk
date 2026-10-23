import { useNavigate } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { currentUserSet, selectCurrentUser } from '../users/usersSlice'
import { signedOut } from './authSlice'

export default function Profile() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const user = useAppSelector(selectCurrentUser)

  function handleSignOut() {
    dispatch(signedOut())
    dispatch(currentUserSet(null))
    navigate('/login', { replace: true })
  }

  if (!user) return <main className="auth-card loading-card">Загружаем профиль…</main>

  return (
    <main className="auth-card profile-card">
      <header className="profile-header">
        <div>
          <p className="eyebrow">Моя информация</p>
          <h1>{user.name}</h1>
        </div>
        <span className={`account-status ${user.activated ? 'account-status--active' : ''}`}>
          {user.activated ? 'Активирован' : 'Не активирован'}
        </span>
      </header>

      <dl className="profile-list">
        <div><dt>ID пользователя</dt><dd>{user.id}</dd></div>
        <div><dt>Имя</dt><dd>{user.name}</dd></div>
        <div><dt>Электронная почта</dt><dd>{user.email}</dd></div>
      </dl>

      <div className="form-divider" aria-hidden="true" />
      <div className="form-actions">
        <button className="secondary-button" type="button" onClick={handleSignOut}>Выйти</button>
      </div>
    </main>
  )
}
