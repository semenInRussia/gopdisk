const assert = require('node:assert/strict')
const fs = require('node:fs')
const test = require('node:test')
const ts = require('typescript')
const { configureStore } = require('@reduxjs/toolkit')

require.extensions['.ts'] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  module._compile(outputText, filename)
}

function storage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
    clear: () => values.clear(),
  }
}
globalThis.window = { localStorage: storage(), sessionStorage: storage() }
const { TOKEN_KEY, readSessionToken, saveSessionToken } = require('../src/features/auth/session.ts')
const { default: auth, login, restoreSession, signedOut, sessionTokenChanged } = require('../src/features/auth/authSlice.ts')
const { default: users } = require('../src/features/users/usersSlice.ts')
const makeStore = () => configureStore({ reducer: { auth, users } })
const user = (id) => ({ id, name: id, email: `${id}@example.com`, activated: true })

test.beforeEach(() => {
  window.localStorage.clear()
  window.sessionStorage = storage()
})

test('migrates the existing session and makes it available to a new tab', () => {
  window.sessionStorage.setItem(TOKEN_KEY, 'existing-token')
  assert.equal(readSessionToken(), 'existing-token')
  assert.equal(window.sessionStorage.getItem(TOKEN_KEY), null)
  window.sessionStorage = storage()
  assert.equal(readSessionToken(), 'existing-token')
})

test('does not revive an old tab session after shared logout', () => {
  saveSessionToken('token')
  saveSessionToken(null)
  window.sessionStorage.setItem(TOKEN_KEY, 'old-token')
  assert.equal(readSessionToken(), null)
  assert.equal(window.sessionStorage.getItem(TOKEN_KEY), null)
})

test('login persists a token that restores the same user in another tab', async (t) => {
  const account = user('owner')
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    if (path === '/v1/tokens/authentication') return Response.json({ authentication_token: { token: 'shared-token' } })
    assert.equal(path, '/v1/me')
    assert.equal(options.headers.get('Authorization'), 'Bearer shared-token')
    return Response.json({ user: account })
  })
  const first = makeStore()
  await first.dispatch(login({ email: account.email, password: 'password' })).unwrap()
  window.sessionStorage = storage()
  const second = makeStore()
  second.dispatch(sessionTokenChanged(readSessionToken()))
  await second.dispatch(restoreSession()).unwrap()
  assert.equal(second.getState().auth.sessionStatus, 'authenticated')
  assert.equal(second.getState().users.currentUserId, first.getState().users.currentUserId)
  first.dispatch(signedOut())
  second.dispatch(sessionTokenChanged(readSessionToken()))
  assert.equal(second.getState().auth.sessionStatus, 'anonymous')
  assert.equal(second.getState().auth.token, null)
})

for (const rejected of [false, true]) {
  test(`ignores a late ${rejected ? 'failed' : 'successful'} restore after switching users`, async (t) => {
    let completeOld
    t.mock.method(globalThis, 'fetch', async (_path, options) => {
      if (options.headers.get('Authorization') === 'Bearer old-token') {
        return new Promise((resolve) => { completeOld = resolve })
      }
      return Response.json({ user: user('new-owner') })
    })
    const store = makeStore()
    saveSessionToken('old-token')
    store.dispatch(sessionTokenChanged('old-token'))
    const oldRequest = store.dispatch(restoreSession())
    saveSessionToken('new-token')
    store.dispatch(sessionTokenChanged('new-token'))
    await store.dispatch(restoreSession()).unwrap()
    completeOld(rejected ? Response.json({ error: 'Expired' }, { status: 401 }) : Response.json({ user: user('old-owner') }))
    await oldRequest
    assert.equal(store.getState().auth.token, 'new-token')
    assert.equal(store.getState().auth.sessionStatus, 'authenticated')
    assert.equal(store.getState().users.currentUserId, 'new-owner')
    assert.equal(window.localStorage.getItem(TOKEN_KEY), 'new-token')
  })
}

test('does not restore a user after logout while a request is in progress', async (t) => {
  let complete
  t.mock.method(globalThis, 'fetch', () => new Promise((resolve) => { complete = resolve }))
  const store = makeStore()
  saveSessionToken('token')
  store.dispatch(sessionTokenChanged('token'))
  const request = store.dispatch(restoreSession())
  store.dispatch(signedOut())
  complete(Response.json({ user: user('owner') }))
  await request
  assert.equal(store.getState().auth.sessionStatus, 'anonymous')
  assert.equal(store.getState().users.currentUserId, null)
  assert.equal(readSessionToken(), null)
})
