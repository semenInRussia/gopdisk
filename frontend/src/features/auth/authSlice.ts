import { createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { apiRequest, serializeApiError, type ApiError } from '../../api/client'
import type { RootState } from '../../app/store'
import { currentUserSet, type User } from '../users/usersSlice'

import { readSessionToken, saveSessionToken, TOKEN_KEY } from './session'

interface AuthState {
  token: string | null
  sessionStatus: 'checking' | 'anonymous' | 'authenticated'
  requestStatus: 'idle' | 'loading'
  error: ApiError | null
  message: string | null
  restoreRequestId: string | null
}

const savedToken = readSessionToken()

const initialState: AuthState = {
  token: savedToken,
  sessionStatus: savedToken ? 'checking' : 'anonymous',
  requestStatus: 'idle',
  error: null,
  message: null,
  restoreRequestId: null,
}

interface AuthenticationEnvelope {
  authentication_token: {
    token: string
    expiry: string
  }
}

interface UserEnvelope {
  user: User
}

export const login = createAsyncThunk<
  string,
  { email: string; password: string },
  { dispatch: import('../../app/store').AppDispatch; rejectValue: ApiError }
>('auth/login', async (credentials, { dispatch, rejectWithValue }) => {
  try {
    const authentication = await apiRequest<AuthenticationEnvelope>('/v1/tokens/authentication', {
      method: 'POST',
      body: JSON.stringify(credentials),
    })
    const token = authentication.authentication_token.token
    const profile = await apiRequest<UserEnvelope>('/v1/me', {}, token)
    saveSessionToken(token)
    dispatch(currentUserSet(profile.user))
    return token
  } catch (error) {
    return rejectWithValue(serializeApiError(error))
  }
})

export const registerAccount = createAsyncThunk<
  string,
  { name: string; email: string; password: string },
  { rejectValue: ApiError }
>('auth/registerAccount', async (details, { rejectWithValue }) => {
  try {
    const response = await apiRequest<{ message: string }>('/v1/users', {
      method: 'POST',
      body: JSON.stringify(details),
    })
    return response.message
  } catch (error) {
    return rejectWithValue(serializeApiError(error))
  }
})

export const restoreSession = createAsyncThunk<
  User,
  void,
  { state: RootState; dispatch: import('../../app/store').AppDispatch; rejectValue: ApiError }
>('auth/restoreSession', async (_, { dispatch, getState, rejectWithValue, requestId }) => {
  const token = getState().auth.token
  if (!token) return rejectWithValue({ message: 'Сохранённая сессия отсутствует.' })

  try {
    const profile = await apiRequest<UserEnvelope>('/v1/me', {}, token)
    if (getState().auth.restoreRequestId === requestId && getState().auth.token === token && window.localStorage.getItem(TOKEN_KEY) === token) {
      dispatch(currentUserSet(profile.user))
    }
    return profile.user
  } catch (error) {
    return rejectWithValue(serializeApiError(error))
  }
})

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    sessionTokenChanged(state, action: PayloadAction<string | null>) {
      state.token = action.payload
      state.sessionStatus = action.payload ? 'checking' : 'anonymous'
      state.restoreRequestId = null
      state.requestStatus = 'idle'
      state.error = null
      state.message = null
    },
    feedbackCleared(state) {
      state.error = null
      state.message = null
    },
    signedOut(state) {
      saveSessionToken(null)
      state.token = null
      state.sessionStatus = 'anonymous'
      state.requestStatus = 'idle'
      state.error = null
      state.message = null
      state.restoreRequestId = null
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.pending, (state) => {
        state.requestStatus = 'loading'
        state.error = null
      })
      .addCase(login.fulfilled, (state, action) => {
        state.token = action.payload
        state.sessionStatus = 'authenticated'
        state.requestStatus = 'idle'
      })
      .addCase(login.rejected, (state, action) => {
        state.requestStatus = 'idle'
        state.error = action.payload ?? { message: 'Не удалось войти.' }
      })
      .addCase(registerAccount.pending, (state) => {
        state.requestStatus = 'loading'
        state.error = null
        state.message = null
      })
      .addCase(registerAccount.fulfilled, (state) => {
        state.requestStatus = 'idle'
        state.message = 'Аккаунт создан. Проверьте почту и активируйте его перед входом.'
      })
      .addCase(registerAccount.rejected, (state, action) => {
        state.requestStatus = 'idle'
        state.error = action.payload ?? { message: 'Не удалось зарегистрироваться.' }
      })
      .addCase(restoreSession.pending, (state, action) => {
        state.sessionStatus = 'checking'
        state.restoreRequestId = action.meta.requestId
      })
      .addCase(restoreSession.fulfilled, (state, action) => {
        if (state.restoreRequestId !== action.meta.requestId) return
        state.sessionStatus = 'authenticated'
        state.restoreRequestId = null
      })
      .addCase(restoreSession.rejected, (state, action) => {
        if (state.restoreRequestId !== action.meta.requestId) return
        if (window.localStorage.getItem(TOKEN_KEY) === state.token) saveSessionToken(null)
        state.token = null
        state.sessionStatus = 'anonymous'
        state.restoreRequestId = null
      })
  },
})

export const { feedbackCleared, signedOut, sessionTokenChanged } = authSlice.actions
export default authSlice.reducer
