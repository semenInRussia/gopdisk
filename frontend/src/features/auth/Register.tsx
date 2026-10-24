import { useEffect, useRef, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { feedbackCleared, registerAccount } from './authSlice'
import StatusMessage from './StatusMessage'

export default function Register() {
  const dispatch = useAppDispatch()
  const passwordRef = useRef<HTMLInputElement>(null)
  const repeatPasswordRef = useRef<HTMLInputElement>(null)
  const { requestStatus, error, message } = useAppSelector((state) => state.auth)

  useEffect(() => {
    dispatch(feedbackCleared())
  }, [dispatch])

  function validatePasswords() {
    const password = passwordRef.current?.value ?? ''
    const repeated = repeatPasswordRef.current?.value ?? ''
    repeatPasswordRef.current?.setCustomValidity(
      password === repeated ? '' : 'Пароли не совпадают',
    )
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    validatePasswords()
    if (!event.currentTarget.checkValidity()) {
      event.currentTarget.reportValidity()
      return
    }

    const form = new FormData(event.currentTarget)
    dispatch(registerAccount({
      name: String(form.get('name') ?? ''),
      email: String(form.get('email') ?? ''),
      password: String(form.get('password') ?? ''),
    }))
  }

  return (
    <main className="auth-card registration-card">
      <header className="auth-header">
        <h1>Создание аккаунта</h1>
        <p>Личное пространство для ваших файлов.</p>
      </header>
      <StatusMessage error={error} message={message} />

      {!message && (
        <form onSubmit={handleSubmit} aria-busy={requestStatus === 'loading'}>
          <div className="form-grid">
            <div className="form-field">
              <label className="form-label" htmlFor="name">Имя пользователя</label>
              <input className="p-field p-field-touch" id="name" name="name" autoComplete="nickname" aria-invalid={Boolean(error?.fields?.name) || undefined} required />
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="register-email">Электронная почта</label>
              <input className="p-field p-field-touch" id="register-email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} placeholder="name@example.com" aria-invalid={Boolean(error?.fields?.email) || undefined} required />
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="new-password">Пароль</label>
              <input className="p-field p-field-touch" id="new-password" name="password" type="password" autoComplete="new-password" minLength={8} maxLength={72} aria-describedby="password-hint" aria-invalid={Boolean(error?.fields?.password) || undefined} required ref={passwordRef} onInput={validatePasswords} />
              <small className="field-hint" id="password-hint">От 8 до 72 символов</small>
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="repeat-password">Повторите пароль</label>
              <input className="p-field p-field-touch" id="repeat-password" name="repeatPassword" type="password" autoComplete="new-password" minLength={8} maxLength={72} required ref={repeatPasswordRef} onInput={validatePasswords} />
            </div>
          </div>

          <div className="form-divider" aria-hidden="true" />
          <div className="form-actions form-actions--split">
            <Link className="text-link" to="/login">Уже есть аккаунт</Link>
            <button className="p-button p-button-primary p-button-touch-large" type="submit" disabled={requestStatus === 'loading'}>
              {requestStatus === 'loading' ? 'Создаём…' : 'Создать аккаунт'}
            </button>
          </div>
        </form>
      )}

      {message && (
        <div className="success-actions">
          <Link className="p-button p-button-primary p-button-touch-large" to="/login">Перейти ко входу</Link>
        </div>
      )}
    </main>
  )
}
