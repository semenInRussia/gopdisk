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
    <main className="p-glass auth-card auth-card--compact">
      <header className="auth-header">
        <h1>Вход в GopherDisk</h1>
        <p>Ваши файлы под рукой.</p>
      </header>
      <StatusMessage error={error} />

      <form onSubmit={handleSubmit} aria-busy={requestStatus === 'loading'}>
        <div className="form-grid form-grid--single">
          <div className="form-field">
            <label className="form-label" htmlFor="login-email">Электронная почта</label>
            <input className="p-field p-field-touch" id="login-email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} placeholder="name@example.com" aria-invalid={Boolean(error?.fields?.email) || undefined} required />
          </div>
          <div className="form-field">
            <label className="form-label" htmlFor="login-password">Пароль</label>
            <input className="p-field p-field-touch" id="login-password" name="password" type="password" autoComplete="current-password" aria-invalid={Boolean(error?.fields?.password) || undefined} required />
          </div>
        </div>

        <div className="form-divider" aria-hidden="true" />
        <div className="form-actions form-actions--split">
          <Link className="text-link" to="/register">Создать аккаунт</Link>
          <button className="p-button p-button-glass-prominent p-button-touch-large" type="submit" disabled={requestStatus === 'loading'}>
            {requestStatus === 'loading' ? 'Входим…' : 'Войти'}
          </button>
        </div>
      </form>
    </main>
  )
}
