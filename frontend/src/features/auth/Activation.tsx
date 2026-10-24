import { useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from '../../app/hooks'
import { activateAccount, activationReset } from './activationSlice'
import StatusMessage from './StatusMessage'

export default function Activation() {
  const dispatch = useAppDispatch()
  const [params, setParams] = useSearchParams()
  const { status, error } = useAppSelector((state) => state.activation)
  const tokens = params.getAll('token')
  const token = tokens[0] ?? ''
  const validToken = tokens.length === 1 && token.length === 26
  const succeeded = status === 'succeeded'

  useEffect(() => {
    dispatch(activationReset())
    return () => { dispatch(activationReset()) }
  }, [dispatch])

  useEffect(() => {
    if (succeeded && params.has('token')) {
      const next = new URLSearchParams(params)
      next.delete('token')
      setParams(next, { replace: true })
    }
  }, [succeeded, params, setParams])

  return (
    <main className="auth-card">
      <header className="auth-header">
        <h1>Подтверждение email</h1>
        <p>{succeeded ? 'Теперь вы можете войти в GopherDisk.' : 'Подтвердите адрес электронной почты, чтобы активировать аккаунт GopherDisk.'}</p>
      </header>
      <StatusMessage
        message={succeeded ? 'Аккаунт активирован.' : null}
        error={!succeeded && (!validToken || error) ? { message: !validToken ? 'Ссылка активации некорректна. Откройте ссылку из письма.' : error! } : null}
      />
      {succeeded || !validToken ? (
        <div className="success-actions"><Link className="p-button p-button-primary p-button-touch-large" to="/login">Перейти ко входу</Link></div>
      ) : (
        <form aria-busy={status === 'loading'} onSubmit={(event) => {
          event.preventDefault()
          void dispatch(activateAccount(token))
        }}>
          <div className="form-actions form-actions--split">
            <Link className="text-link" to="/login">Не подтверждать</Link>
            <button className="p-button p-button-primary p-button-touch-large" type="submit" disabled={status === 'loading'}>
              {status === 'loading' ? 'Активируем…' : 'Активировать аккаунт'}
            </button>
          </div>
        </form>
      )}
    </main>
  )
}
