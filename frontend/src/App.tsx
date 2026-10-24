import { useEffect } from 'react'
import { Navigate, Route, Routes, useSearchParams } from 'react-router-dom'
import { useAppDispatch, useAppSelector } from './app/hooks'
import { restoreSession, sessionTokenChanged } from './features/auth/authSlice'
import { TOKEN_KEY } from './features/auth/session'
import { filesCleared } from './features/files/filesSlice'
import { permissionsCleared } from './features/permissions/permissionsSlice'
import { currentUserSet } from './features/users/usersSlice'
import Login from './features/auth/Login'
import Profile from './features/auth/Profile'
import Register from './features/auth/Register'
import Activation from './features/auth/Activation'
import SharedFile from './features/files/SharedFile'

export default function App() {
  const dispatch = useAppDispatch()
  const sessionStatus = useAppSelector((state) => state.auth.sessionStatus)
  const token = useAppSelector((state) => state.auth.token)
  const [params] = useSearchParams()
  const linkedFile = params.get('file')
  const loginDestination = linkedFile ? `/files/${encodeURIComponent(linkedFile)}` : '/profile'

  useEffect(() => {
    if (sessionStatus === 'checking') dispatch(restoreSession())
  }, [dispatch, sessionStatus, token])

  useEffect(() => {
    function synchronizeSession(event: StorageEvent) {
      if (event.storageArea !== window.localStorage || (event.key !== TOKEN_KEY && event.key !== null)) return
      const nextToken = window.localStorage.getItem(TOKEN_KEY)
      if (nextToken === token) return
      window.sessionStorage.removeItem(TOKEN_KEY)
      dispatch(currentUserSet(null))
      dispatch(filesCleared())
      dispatch(permissionsCleared())
      dispatch(sessionTokenChanged(nextToken))
    }
    window.addEventListener('storage', synchronizeSession)
    return () => window.removeEventListener('storage', synchronizeSession)
  }, [dispatch, token])

  const authenticated = sessionStatus === 'authenticated'
  const loading = <main className="auth-card loading-card" role="status"><span className="p-spinner p-spinner-small" aria-hidden="true" />Восстанавливаем сессию…</main>

  return (
    <Routes>
      <Route path="/activation" element={<Activation />} />
      <Route path="*" element={sessionStatus === 'checking' ? loading : <SessionRoutes authenticated={authenticated} loginDestination={loginDestination} />} />
    </Routes>
  )
}

function SessionRoutes({ authenticated, loginDestination }: { authenticated: boolean; loginDestination: string }) {
  return (
    <Routes>
      <Route path="/login" element={authenticated ? <Navigate to={loginDestination} replace /> : <Login />} />
      <Route path="/files/:id" element={<SharedFile />} />
      <Route path="/register" element={authenticated ? <Navigate to="/profile" replace /> : <Register />} />
      <Route path="/profile" element={authenticated ? <Profile /> : <Navigate to="/login" replace />} />
      <Route path="*" element={<Navigate to={authenticated ? '/profile' : '/login'} replace />} />
    </Routes>
  )
}
