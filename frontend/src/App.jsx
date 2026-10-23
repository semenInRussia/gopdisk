import { useRef } from 'react'

export default function App() {
  const passwordRef = useRef(null)
  const repeatPasswordRef = useRef(null)

  function validatePasswords() {
    const password = passwordRef.current.value
    const repeatPassword = repeatPasswordRef.current.value
    repeatPasswordRef.current.setCustomValidity(
      password === repeatPassword ? '' : 'Пароли не совпадают',
    )
  }

  function handleSubmit(event) {
    event.preventDefault()
    validatePasswords()
  }

  return (
    <form className="registration-form" onSubmit={handleSubmit}>
      <div className="form-grid">
        <div className="form-field">
          <label className="visually-hidden" htmlFor="username">Имя пользователя</label>
          <input
            id="username"
            name="username"
            autoComplete="nickname"
            autoCapitalize="none"
            placeholder="Имя пользователя"
            spellCheck="false"
            required
          />
        </div>

        <div className="form-field">
          <label className="visually-hidden" htmlFor="email">Электронная почта</label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            autoCapitalize="none"
            placeholder="Электронная почта"
            spellCheck="false"
            required
          />
        </div>

        <div className="form-field">
          <label className="visually-hidden" htmlFor="password">Пароль</label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="new-password"
            minLength="8"
            placeholder="Пароль"
            required
            ref={passwordRef}
            onInput={validatePasswords}
          />
        </div>

        <div className="form-field">
          <label className="visually-hidden" htmlFor="repeat-password">Повторите пароль</label>
          <input
            id="repeat-password"
            name="repeatPassword"
            type="password"
            autoComplete="new-password"
            minLength="8"
            placeholder="Повторите пароль"
            required
            ref={repeatPasswordRef}
            onInput={validatePasswords}
          />
        </div>
      </div>

      <div className="form-divider" aria-hidden="true" />

      <div className="form-actions">
        <button type="submit">Продолжить</button>
      </div>
    </form>
  )
}
