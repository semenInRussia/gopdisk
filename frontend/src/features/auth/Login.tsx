import { useEffect, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { feedbackCleared, login } from './authSlice'
import StatusMessage from './StatusMessage'

export default function Login() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { requestStatus, error } = useAppSelector((state) => state.auth)

  useEffect(() => {
    dispatch(feedbackCleared())
  }, [dispatch])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)

    try {
      await dispatch(login({
        email: String(form.get('email') ?? ''),
        password: String(form.get('password') ?? ''),
      })).unwrap()
      navigate('/profile', { replace: true })
    } catch {
      // The slice exposes the normalized API error.
    }
  }

  return (
    <main className="auth-card auth-card--compact">
      <header className="auth-header"><h1>Вход в GopherDisk</h1></header>
      <StatusMessage error={error} />

      <form onSubmit={handleSubmit}>
        <div className="form-grid form-grid--single">
          <div className="form-field">
            <label className="visually-hidden" htmlFor="login-email">Электронная почта</label>
            <input id="login-email" name="email" type="email" autoComplete="username" placeholder="Электронная почта" required />
          </div>
          <div className="form-field">
            <label className="visually-hidden" htmlFor="login-password">Пароль</label>
            <input id="login-password" name="password" type="password" autoComplete="current-password" placeholder="Пароль" required />
          </div>
        </div>

        <div className="form-divider" aria-hidden="true" />
        <div className="form-actions form-actions--split">
          <Link className="text-link" to="/register">Создать аккаунт</Link>
          <button type="submit" disabled={requestStatus === 'loading'}>
            {requestStatus === 'loading' ? 'Входим…' : 'Войти'}
          </button>
        </div>
      </form>
    </main>
  )
}
