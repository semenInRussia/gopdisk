import { createEntityAdapter, createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { RootState } from '../../app/store'

export interface User {
  id: string
  name: string
  email: string
  activated: boolean
}

const usersAdapter = createEntityAdapter<User>({
  sortComparer: (left, right) => left.name.localeCompare(right.name),
})

const initialState = usersAdapter.getInitialState({
  currentUserId: null as string | null,
})

const usersSlice = createSlice({
  name: 'users',
  initialState,
  reducers: {
    usersReceived: usersAdapter.setAll,
    userUpserted: usersAdapter.upsertOne,
    userRemoved(state, action: PayloadAction<string>) {
      usersAdapter.removeOne(state, action.payload)
      if (state.currentUserId === action.payload) state.currentUserId = null
    },
    currentUserSet(state, action: PayloadAction<User | null>) {
      const user = action.payload
      if (user) usersAdapter.upsertOne(state, user)
      state.currentUserId = user?.id ?? null
    },
    usersCleared(state) {
      usersAdapter.removeAll(state)
      state.currentUserId = null
    },
  },
})

export const { currentUserSet, userRemoved, userUpserted, usersCleared, usersReceived } =
  usersSlice.actions

const userSelectors = usersAdapter.getSelectors((state: RootState) => state.users)

export const selectAllUsers = userSelectors.selectAll
export const selectUserById = userSelectors.selectById
export const selectCurrentUser = (state: RootState) =>
  state.users.currentUserId ? userSelectors.selectById(state, state.users.currentUserId) : null

export default usersSlice.reducer
