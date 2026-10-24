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

async function activate(label) {
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
  await act(async () => button.dispatchEvent(new window.MouseEvent('click', { bubbles: true })))
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
