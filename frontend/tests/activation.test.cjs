const assert = require('node:assert/strict')
const fs = require('node:fs')
const test = require('node:test')
const ts = require('typescript')
const { JSDOM } = require('jsdom')
const { configureStore } = require('@reduxjs/toolkit')

for (const extension of ['.ts', '.tsx']) {
  require.extensions[extension] = (module, filename) => {
    const { outputText } = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
    })
    module._compile(outputText, filename)
  }
}

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.IS_REACT_ACT_ENVIRONMENT = true
const React = require('react')
const { act } = React
const { createRoot } = require('react-dom/client')
const { Provider } = require('react-redux')
const { MemoryRouter, Route, Routes, useLocation } = require('react-router-dom')
const { default: Activation } = require('../src/features/auth/Activation.tsx')
const { default: activation, activateAccount } = require('../src/features/auth/activationSlice.ts')
const { default: auth, sessionTokenChanged } = require('../src/features/auth/authSlice.ts')
const { default: App } = require('../src/App.tsx')
const { default: files } = require('../src/features/files/filesSlice.ts')
const { default: users } = require('../src/features/users/usersSlice.ts')
const { default: permissions } = require('../src/features/permissions/permissionsSlice.ts')
const token = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'

function Location() {
  const location = useLocation()
  return React.createElement('output', null, location.pathname + location.search)
}

async function mount(t, query = `?token=${token}`, session = null) {
  const store = configureStore({ reducer: { activation, auth } })
  if (session) store.dispatch(sessionTokenChanged(session))
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  await act(async () => root.render(React.createElement(React.StrictMode, null,
    React.createElement(Provider, { store }, React.createElement(MemoryRouter, { initialEntries: [`/activation${query}`] },
      React.createElement(Location),
      React.createElement(Routes, null,
        React.createElement(Route, { path: '/activation', element: React.createElement(Activation) }),
        React.createElement(Route, { path: '/login', element: React.createElement('p', null, 'Вход') }),
      ),
    )),
  )))
  return { host, store }
}

test('opening in StrictMode and declining never sends activation requests', async (t) => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => { throw new Error('unexpected request') })
  const { host } = await mount(t)
  assert.equal(fetch.mock.callCount(), 0)
  await act(async () => host.querySelector('a').click())
  assert.equal(host.querySelector('output').textContent, '/login')
  assert.equal(fetch.mock.callCount(), 0)
})

for (const query of ['', '?token=', '?token=short', `?token=${token}&token=${token}`]) {
  test(`invalid link ${query} cannot submit`, async (t) => {
    const fetch = t.mock.method(globalThis, 'fetch', async () => { throw new Error('unexpected request') })
    const { host } = await mount(t, query)
    assert.match(host.querySelector('[role="alert"]').textContent, /некорректна/)
    assert.equal(host.querySelector('button'), null)
    assert.equal(fetch.mock.callCount(), 0)
  })
}

test('explicit confirmation sends one PUT, removes token and preserves the existing session', async (t) => {
  let finish
  const fetch = t.mock.method(globalThis, 'fetch', (path, options) => {
    assert.equal(path, '/v1/users/activated')
    assert.equal(options.method, 'PUT')
    assert.deepEqual(JSON.parse(options.body), { token })
    assert.equal(options.headers.has('Authorization'), false)
    return new Promise((resolve) => { finish = resolve })
  })
  const { host, store } = await mount(t, `?token=${token}`, 'existing-session')
  const originalAuth = store.getState().auth
  await act(async () => {
    host.querySelector('button').click()
    // Even two events before React rerenders must result in a single request.
    host.querySelector('form').dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true }))
  })
  assert.equal(fetch.mock.callCount(), 1)
  assert.equal(host.querySelector('button').disabled, true)
  await act(async () => { finish(Response.json({ user: { activated: true } })); await new Promise(setImmediate) })
  assert.match(host.textContent, /Аккаунт активирован/)
  assert.equal(host.querySelector('output').textContent, '/activation')
  assert.deepEqual(store.getState().auth, originalAuth)
  assert.equal(host.querySelector('button'), null)
})

for (const failure of ['invalid', 'network', 'server']) {
  test(`${failure} error is shown and a failed request can be retried`, async (t) => {
    let attempts = 0
    t.mock.method(globalThis, 'fetch', async () => {
      attempts++
      if (attempts > 1) return Response.json({ user: { activated: true } })
      if (failure === 'network') throw new TypeError('Failed to fetch')
      return Response.json({ error: failure === 'invalid' ? { token: 'is invalid or has expired' } : 'server error' },
        { status: failure === 'invalid' ? 422 : 500 })
    })
    const { host } = await mount(t)
    await act(async () => { host.querySelector('button').click(); await new Promise(setImmediate) })
    assert.match(host.querySelector('[role="alert"]').textContent, failure === 'invalid' ? /недействительна, уже использована/ : /Попробуйте ещё раз/)
    assert.equal(host.querySelector('button').disabled, false)
    await act(async () => { host.querySelector('button').click(); await new Promise(setImmediate) })
    assert.match(host.textContent, /Аккаунт активирован/)
  })
}

test('a response arriving after leaving the page does not restore activation state', async (t) => {
  let finish
  t.mock.method(globalThis, 'fetch', () => new Promise((resolve) => { finish = resolve }))
  const { host, store } = await mount(t)
  let request
  await act(async () => { request = store.dispatch(activateAccount(token)) })
  await act(async () => host.querySelector('a').click())
  await act(async () => { finish(Response.json({ user: {} })); await request })
  assert.equal(store.getState().activation.status, 'idle')
})

test('App keeps activation public while restoring a saved session and routes decline to login', async (t) => {
  const restores = []
  const fetch = t.mock.method(globalThis, 'fetch', (path) => {
    assert.equal(path, '/v1/me')
    return new Promise((resolve) => restores.push(resolve))
  })
  const store = configureStore({ reducer: { activation, auth, files, users, permissions } })
  store.dispatch(sessionTokenChanged('saved-session'))
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  await act(async () => root.render(React.createElement(React.StrictMode, null,
    React.createElement(Provider, { store }, React.createElement(MemoryRouter, { initialEntries: [`/activation?token=${token}`] },
      React.createElement(App),
    )),
  )))
  assert.equal(host.querySelector('h1').textContent, 'Подтверждение email')
  assert.equal(store.getState().auth.sessionStatus, 'checking')
  await act(async () => {
    restores.forEach((resolve) => resolve(Response.json({ error: 'expired' }, { status: 401 })))
    await new Promise(setImmediate)
  })
  assert.equal(host.querySelector('h1').textContent, 'Подтверждение email')
  const requestsBeforeDecline = fetch.mock.callCount()
  await act(async () => host.querySelector('a').click())
  assert.equal(host.querySelector('input[type="password"]').name, 'password')
  assert.equal(fetch.mock.callCount(), requestsBeforeDecline)
})
