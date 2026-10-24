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
globalThis.Node = dom.window.Node
globalThis.HTMLElement = dom.window.HTMLElement
globalThis.IS_REACT_ACT_ENVIRONMENT = true
window.matchMedia = () => ({ matches: true, addEventListener() {}, removeEventListener() {} })
globalThis.ResizeObserver = globalThis.IntersectionObserver = class {
  observe() {}
  disconnect() {}
}
window.HTMLDialogElement.prototype.showModal = function () { this.open = true }
window.HTMLDialogElement.prototype.close = function () { this.open = false; this.dispatchEvent(new window.Event('close')) }

const React = require('react')
const { act } = React
const { createRoot } = require('react-dom/client')
const { Provider } = require('react-redux')
const { default: FileBrowser } = require('../src/features/files/FileBrowser.tsx')
const { default: filesReducer, filesReceived } = require('../src/features/files/filesSlice.ts')
const { default: usersReducer, currentUserSet } = require('../src/features/users/usersSlice.ts')
const { default: permissionsReducer } = require('../src/features/permissions/permissionsSlice.ts')
const actions = require('../src/features/files/fileActions.ts')
const permissionsApi = require('../src/features/permissions/permissionsApi.ts')

const file = { id: 'file-id', owner_id: 'owner-id', name: 'document.txt', type: 'text/plain', size: 5, status: 'ready' }

async function render(t) {
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  const store = configureStore({ reducer: { files: filesReducer, users: usersReducer, permissions: permissionsReducer } })
  store.dispatch(filesReceived([file]))
  store.dispatch(currentUserSet({ id: file.owner_id, name: 'Owner', email: 'owner@example.com', activated: true }))
  await act(async () => root.render(React.createElement(Provider, { store }, React.createElement(FileBrowser, {
    files: [file], viewMode: 'grid', token: 'token', onError: (message) => { if (message) assert.fail(message) },
  }))))
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  return store
}

async function openMenu() {
  await act(async () => document.querySelector('[data-file-id]').dispatchEvent(new window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 50, clientY: 50 })))
  assert.ok(document.querySelector('[role="menu"]'))
}

async function activate(label, options = {}) {
  const menu = document.querySelector('[role="menu"]')
  const button = [...menu.querySelectorAll('button')].find((element) => element.textContent === label)
  assert.ok(button)
  await act(async () => {
    const down = new window.MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 })
    button.dispatchEvent(down)
    assert.equal(down.defaultPrevented, true)
    // Reproduce a platform blur with no relatedTarget before the click is delivered.
    document.activeElement.dispatchEvent(new window.FocusEvent('focusout', { bubbles: true, relatedTarget: null }))
  })
  assert.ok(button.isConnected, 'menu item must survive until click')
  await act(async () => button.dispatchEvent(new window.MouseEvent('click', { bubbles: true, ...options })))
}

test('context menu downloads the right-clicked file despite a blur before click', async (t) => {
  const download = t.mock.method(actions, 'downloadStoredFiles', async () => {})
  await render(t)
  await openMenu()
  await activate('Скачать')
  assert.equal(download.mock.callCount(), 1)
  assert.deepEqual(download.mock.calls[0].arguments.slice(0, 2), [[file], 'token'])
  assert.equal(document.querySelector('[role="menu"]'), null)
})

test('context menu deletes the right-clicked file despite a blur before click', async (t) => {
  const remove = t.mock.method(actions, 'deleteStoredFile', async () => {})
  t.mock.method(window, 'confirm', () => true)
  const store = await render(t)
  await openMenu()
  await activate('Удалить')
  assert.equal(remove.mock.callCount(), 1)
  assert.deepEqual(remove.mock.calls[0].arguments, [file.id, 'token'])
  assert.deepEqual(store.getState().files.ids, [])
})

test('file falls only after server deletion succeeds and is removed after the animation', async (t) => {
  let acknowledge
  t.mock.method(actions, 'deleteStoredFile', () => new Promise((resolve) => { acknowledge = resolve }))
  t.mock.method(window, 'confirm', () => true)
  t.mock.method(window, 'matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
  const animations = []
  const prototype = window.HTMLElement.prototype
  prototype.animate = function (frames, options) {
    const animation = { element: this, frames, options, finish() { this.onfinish?.() } }
    animations.push(animation)
    return animation
  }
  t.after(() => { delete prototype.animate })
  const store = await render(t)
  await openMenu()
  await activate('Удалить')
  assert.equal(animations.length, 0)
  assert.deepEqual(store.getState().files.ids, [file.id])
  await act(async () => acknowledge())
  assert.equal(animations.length, 1)
  assert.equal(animations[0].element.tagName, 'LI')
  assert.ok(animations[0].element.classList.contains('file-removing'))
  assert.equal(animations[0].frames.at(-1).transform, 'translateY(110px) rotate(9deg) scale(0.92)')
  assert.deepEqual(store.getState().files.ids, [file.id])
  await act(async () => animations[0].finish())
  assert.deepEqual(store.getState().files.ids, [])
})

test('context menu opens permissions despite a blur before click', async (t) => {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  const load = t.mock.method(permissionsApi, 'loadFilePermissions', async () => [])
  await render(t)
  await openMenu()
  await activate('Права доступа')
  assert.ok(document.querySelector('dialog[open]'))
  assert.equal(document.querySelector('dialog h2').textContent, 'Права доступа')
  assert.equal(load.mock.callCount(), 1)
})

test('context menu still closes on an outside click and on keyboard focus leaving it', async (t) => {
  await render(t)
  await openMenu()
  await act(async () => document.body.dispatchEvent(new window.MouseEvent('pointerdown', { bubbles: true })))
  assert.equal(document.querySelector('[role="menu"]'), null)
  await openMenu()
  await act(async () => document.activeElement.dispatchEvent(new window.FocusEvent('focusout', { bubbles: true, relatedTarget: document.body })))
  assert.equal(document.querySelector('[role="menu"]'), null)
})

test('permissions window expands from the clicked menu position', async (t) => {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  t.mock.method(permissionsApi, 'loadFilePermissions', async () => [])
  t.mock.method(window, 'matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
  const prototype = window.HTMLElement.prototype
  const originalRect = prototype.getBoundingClientRect
  t.mock.method(prototype, 'getBoundingClientRect', function () {
    return this.tagName === 'DIALOG'
      ? { left: 200, top: 100, width: 400, height: 300 }
      : originalRect.call(this)
  })
  const animations = []
  prototype.animate = function (frames) { animations.push({ element: this, frames }); return { cancel() {} } }
  t.after(() => { delete prototype.animate })
  await render(t)
  await openMenu()
  await activate('Права доступа', { clientX: 120, clientY: 80, detail: 1 })
  assert.equal(animations.length, 1)
  assert.equal(animations[0].element.tagName, 'DIALOG')
  assert.equal(animations[0].frames[0].transform, 'translate(-280px, -170px) scale(0.08)')
  assert.equal(animations[0].frames[1].transform, 'none')
})

test('permissions closes back to its opening point and stays mounted until animation finishes', async (t) => {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  t.mock.method(permissionsApi, 'loadFilePermissions', async () => [])
  t.mock.method(window, 'matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
  const animations = []
  const prototype = window.HTMLElement.prototype
  prototype.animate = function (frames) {
    const animation = {
      frames, playState: 'running',
      cancel() { this.playState = 'idle' },
      finish() { this.playState = 'finished'; this.onfinish?.() },
    }
    animations.push(animation)
    return animation
  }
  t.after(() => { delete prototype.animate })
  await render(t)
  await openMenu()
  await activate('Права доступа', { clientX: 120, clientY: 80, detail: 1 })
  await act(async () => animations[0].finish())
  const dialog = document.querySelector('dialog[open]')
  await act(async () => dialog.querySelector('[aria-label="Закрыть окно общего доступа"]').click())
  assert.ok(dialog.isConnected && dialog.open)
  assert.equal(animations.length, 2)
  assert.equal(animations[1].frames[1].transform, 'translate(120px, 80px) scale(0.08)')
  await act(async () => animations[1].finish())
  assert.equal(document.querySelector('dialog'), null)
})

test('clicking the user button again closes its panel even when the browser blurs without a target', async (t) => {
  const { MemoryRouter } = require('react-router-dom')
  const { default: Profile } = require('../src/features/auth/Profile.tsx')
  const { loadFiles } = require('../src/features/files/filesSlice.ts')
  const store = configureStore({ reducer: {
    files: filesReducer, users: usersReducer, permissions: permissionsReducer,
    auth: () => ({ token: 'token' }),
  } })
  store.dispatch(currentUserSet({ id: file.owner_id, name: 'Owner', email: 'owner@example.com', activated: true }))
  store.dispatch(loadFiles.fulfilled([file], 'loaded', file.owner_id))
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  await act(async () => root.render(React.createElement(Provider, { store }, React.createElement(MemoryRouter, null, React.createElement(Profile)))))
  const trigger = document.querySelector('[aria-label="Информация о пользователе"]')
  await act(async () => trigger.click())
  assert.equal(trigger.getAttribute('aria-expanded'), 'true')
  assert.ok(document.querySelector('#user-info'))
  await act(async () => {
    const down = new window.MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 })
    trigger.dispatchEvent(down)
    assert.equal(down.defaultPrevented, true)
    document.querySelector('#user-info').dispatchEvent(new window.FocusEvent('focusout', { bubbles: true, relatedTarget: null }))
    trigger.click()
  })
  assert.equal(trigger.getAttribute('aria-expanded'), 'false')
  assert.equal(document.querySelector('#user-info'), null)
  await act(async () => trigger.click())
  assert.equal(trigger.getAttribute('aria-expanded'), 'true')
  assert.equal(document.querySelectorAll('#user-info').length, 1)
})
