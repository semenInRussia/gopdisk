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
    <main className="auth-card">
      <header className="auth-header"><h1>Создание аккаунта</h1></header>
      <StatusMessage error={error} message={message} />

      {!message && (
        <form onSubmit={handleSubmit}>
          <div className="form-grid">
            <div className="form-field">
              <label className="visually-hidden" htmlFor="name">Имя пользователя</label>
              <input id="name" name="name" autoComplete="nickname" placeholder="Имя пользователя" required />
            </div>
            <div className="form-field">
              <label className="visually-hidden" htmlFor="register-email">Электронная почта</label>
              <input id="register-email" name="email" type="email" autoComplete="username" placeholder="Электронная почта" required />
            </div>
            <div className="form-field">
              <label className="visually-hidden" htmlFor="new-password">Пароль</label>
              <input id="new-password" name="password" type="password" autoComplete="new-password" minLength={8} maxLength={72} placeholder="Пароль" required ref={passwordRef} onInput={validatePasswords} />
            </div>
            <div className="form-field">
              <label className="visually-hidden" htmlFor="repeat-password">Повторите пароль</label>
              <input id="repeat-password" name="repeatPassword" type="password" autoComplete="new-password" minLength={8} maxLength={72} placeholder="Повторите пароль" required ref={repeatPasswordRef} onInput={validatePasswords} />
            </div>
          </div>

          <div className="form-divider" aria-hidden="true" />
          <div className="form-actions form-actions--split">
            <Link className="text-link" to="/login">Уже есть аккаунт</Link>
            <button type="submit" disabled={requestStatus === 'loading'}>
              {requestStatus === 'loading' ? 'Создаём…' : 'Продолжить'}
            </button>
          </div>
        </form>
      )}

      {message && (
        <div className="success-actions">
          <Link className="primary-link" to="/login">Перейти ко входу</Link>
        </div>
      )}
    </main>
  )
}
