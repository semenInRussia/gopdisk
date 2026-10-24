import { useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from './app/hooks'
import { restoreSession } from './features/auth/authSlice'
import Login from './features/auth/Login'
import Profile from './features/auth/Profile'
import Register from './features/auth/Register'

export default function App() {
  const dispatch = useAppDispatch()
  const sessionStatus = useAppSelector((state) => state.auth.sessionStatus)

  useEffect(() => {
    if (sessionStatus === 'checking') dispatch(restoreSession())
  }, [dispatch, sessionStatus])

  if (sessionStatus === 'checking') {
    return <main className="auth-card loading-card" role="status"><span className="p-spinner p-spinner-small" aria-hidden="true" />Восстанавливаем сессию…</main>
  }

  const authenticated = sessionStatus === 'authenticated'

  return (
    <Routes>
      <Route path="/login" element={authenticated ? <Navigate to="/profile" replace /> : <Login />} />
      <Route path="/register" element={authenticated ? <Navigate to="/profile" replace /> : <Register />} />
      <Route path="/profile" element={authenticated ? <Profile /> : <Navigate to="/login" replace />} />
      <Route path="*" element={<Navigate to={authenticated ? '/profile' : '/login'} replace />} />
    </Routes>
  )
}
