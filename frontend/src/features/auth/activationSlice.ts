import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'
import { apiRequest, serializeApiError } from '../../api/client'
import type { RootState } from '../../app/store'

interface ActivationState {
  status: 'idle' | 'loading' | 'succeeded' | 'failed'
  error: string | null
  requestId: string | null
}

const initialState: ActivationState = { status: 'idle', error: null, requestId: null }

export const activateAccount = createAsyncThunk<void, string, { state: RootState; rejectValue: string }>(
  'activation/activateAccount',
  async (token, { rejectWithValue }) => {
    try {
      await apiRequest('/v1/users/activated', { method: 'PUT', body: JSON.stringify({ token }) })
    } catch (error) {
      const details = serializeApiError(error)
      return rejectWithValue(details.fields?.token
        ? 'Ссылка недействительна, уже использована или срок её действия истёк.'
        : 'Не удалось активировать аккаунт. Попробуйте ещё раз.')
    }
  },
  { condition: (_, { getState }) => getState().activation.status !== 'loading' },
)

const activationSlice = createSlice({
  name: 'activation',
  initialState,
  reducers: {
    activationReset: () => initialState,
  },
  extraReducers: (builder) => {
    builder
      .addCase(activateAccount.pending, (state, action) => {
        state.status = 'loading'
        state.error = null
        state.requestId = action.meta.requestId
      })
      .addCase(activateAccount.fulfilled, (state, action) => {
        if (state.requestId !== action.meta.requestId) return
        state.status = 'succeeded'
        state.requestId = null
      })
      .addCase(activateAccount.rejected, (state, action) => {
        if (state.requestId !== action.meta.requestId) return
        state.status = 'failed'
        state.error = action.payload ?? 'Не удалось активировать аккаунт. Попробуйте ещё раз.'
        state.requestId = null
      })
  },
})

export const { activationReset } = activationSlice.actions
export default activationSlice.reducer
