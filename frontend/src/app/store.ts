import { configureStore } from '@reduxjs/toolkit'
import authReducer from '../features/auth/authSlice'
import filesReducer from '../features/files/filesSlice'
import permissionsReducer from '../features/permissions/permissionsSlice'
import usersReducer from '../features/users/usersSlice'

export const store = configureStore({
  reducer: {
    auth: authReducer,
    users: usersReducer,
    files: filesReducer,
    permissions: permissionsReducer,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
