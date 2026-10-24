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
window.AnimationEvent = window.Event
window.matchMedia = () => ({ matches: true, addEventListener() {}, removeEventListener() {} })
globalThis.ResizeObserver = globalThis.IntersectionObserver = class {
  observe() {}
  disconnect() {}
}
window.HTMLDialogElement.prototype.showModal = function () { this.open = true }
window.HTMLDialogElement.prototype.close = function (value = '') { this.returnValue = value; this.open = false; this.dispatchEvent(new window.Event('close')) }

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

async function render(t, files = [file]) {
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  const store = configureStore({ reducer: { files: filesReducer, users: usersReducer, permissions: permissionsReducer } })
  store.dispatch(filesReceived(files))
  store.dispatch(currentUserSet({ id: file.owner_id, name: 'Owner', email: 'owner@example.com', activated: true }))
  await act(async () => root.render(React.createElement(Provider, { store }, React.createElement(FileBrowser, {
    files, viewMode: 'grid', token: 'token', onError: (message) => { if (message) assert.fail(message) },
  }))))
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  return store
}

test('touch taps toggle multiple files and the Open action opens a single selection', async (t) => {
  const opening = t.mock.method(require('../src/features/files/openFile.ts'), 'openFile', async () => {})
  await render(t, [file, { ...file, id: 'second-file', name: 'second.txt' }])
  const buttons = [...document.querySelectorAll('[data-file-id]')]
  const open = document.querySelector('.file-touch-open')
  async function tap(button) {
    await act(async () => {
      const pointer = new window.Event('pointerdown', { bubbles: true })
      Object.defineProperty(pointer, 'pointerType', { value: 'touch' })
      button.dispatchEvent(pointer)
      button.click()
    })
  }
  assert.equal(open.disabled, true)
  await tap(buttons[0])
  assert.equal(open.disabled, false)
  await tap(buttons[1])
  assert.deepEqual(buttons.map((button) => button.getAttribute('aria-pressed')), ['true', 'true'])
  assert.equal(open.disabled, true)
  await tap(buttons[0])
  assert.deepEqual(buttons.map((button) => button.getAttribute('aria-pressed')), ['false', 'true'])
  await act(async () => open.click())
  assert.deepEqual(opening.mock.calls[0].arguments, ['second-file', 'token'])
  assert.equal(opening.mock.callCount(), 1)
  await act(async () => buttons[0].click())
  assert.deepEqual(buttons.map((button) => button.getAttribute('aria-pressed')), ['true', 'false'])
})

async function openMenu() {
  await act(async () => document.querySelector('[data-file-id]').dispatchEvent(new window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 50, clientY: 50 })))
  assert.ok(document.querySelector('[role="menu"]'))
}

function touchEvent(type, options = {}) {
  const event = new window.Event(type, { bubbles: true, cancelable: true })
  Object.assign(event, { pointerType: 'touch', pointerId: 1, isPrimary: true, clientX: 60, clientY: 80 }, options)
  return event
}

function holdClock(t) {
  const pending = new Map()
  let nextId = 0
  t.mock.method(window, 'setTimeout', (callback, delay) => {
    assert.equal(delay, 500)
    pending.set(++nextId, callback)
    return nextId
  })
  t.mock.method(window, 'clearTimeout', (id) => pending.delete(id))
  return {
    pending,
    async finish() {
      await act(async () => {
        for (const [id, callback] of pending) { pending.delete(id); callback() }
      })
    },
  }
}

test('holding a file opens its menu and releasing does not toggle the selection', async (t) => {
  await render(t)
  const clock = holdClock(t)
  const button = document.querySelector('[data-file-id]')
  await act(async () => button.dispatchEvent(touchEvent('pointerdown')))
  assert.equal(document.querySelector('[role="menu"]'), null)
  assert.equal(clock.pending.size, 1)
  await clock.finish()
  assert.ok(document.querySelector('[role="menu"]'))
  assert.equal(button.getAttribute('aria-pressed'), 'true')
  await act(async () => {
    button.dispatchEvent(touchEvent('pointerup'))
    button.click()
  })
  assert.ok(document.querySelector('[role="menu"]'))
  assert.equal(button.getAttribute('aria-pressed'), 'true')
  await act(async () => button.dispatchEvent(touchEvent('pointerdown')))
  await act(async () => { button.dispatchEvent(touchEvent('pointerup')); button.click() })
  assert.equal(button.getAttribute('aria-pressed'), 'false', 'a subsequent normal tap still toggles')
})

test('holding the filename opens a touch menu anchored inside a narrow viewport', async (t) => {
  await render(t)
  const clock = holdClock(t)
  t.mock.method(window.HTMLElement.prototype, 'getBoundingClientRect', function () {
    if (this.classList.contains('file-open')) return { left: 250, right: 314, top: 560, bottom: 672, width: 64, height: 112 }
    return { left: 0, right: 250, top: 0, bottom: 180, width: 250, height: 180 }
  })
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  window.innerWidth = 320
  window.innerHeight = 700
  t.after(() => { window.innerWidth = viewportWidth; window.innerHeight = viewportHeight })
  const filename = document.querySelector('.files-list .file-details strong')
  assert.ok(filename.closest('button[data-file-id]'), 'the filename belongs to the same touch target as its icon')
  await act(async () => filename.dispatchEvent(touchEvent('pointerdown')))
  await clock.finish()
  const menu = document.querySelector('[role="menu"]')
  assert.ok(menu.classList.contains('p-menu-touch'))
  assert.ok(menu.classList.contains('p-menu-touch--context'))
  assert.equal(menu.style.left, '62px')
  assert.equal(menu.style.top, '364px')
})

test('lifting the held finger cannot accidentally activate a menu command', async (t) => {
  const download = t.mock.method(actions, 'downloadStoredFiles', async () => {})
  await render(t)
  const clock = holdClock(t)
  const fileButton = document.querySelector('[data-file-id]')
  await act(async () => fileButton.dispatchEvent(touchEvent('pointerdown')))
  await clock.finish()
  const command = document.querySelector('[role="menu"] button')
  await act(async () => {
    fileButton.dispatchEvent(touchEvent('pointerup'))
    command.dispatchEvent(new window.MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }))
  })
  assert.equal(download.mock.callCount(), 0)
  assert.ok(document.querySelector('[role="menu"]'))
  await act(async () => {
    command.dispatchEvent(touchEvent('pointerdown'))
    command.dispatchEvent(touchEvent('pointerup'))
    command.dispatchEvent(new window.MouseEvent('click', { bubbles: true, detail: 1 }))
  })
  assert.equal(download.mock.callCount(), 1)
})

test('release, movement, cancellation, scrolling, blur and a second finger cancel a pending hold', async (t) => {
  await render(t)
  const clock = holdClock(t)
  const button = document.querySelector('[data-file-id]')
  for (const [target, event] of [
    [button, touchEvent('pointerup')],
    [button, touchEvent('pointermove', { clientX: 90 })],
    [button, touchEvent('pointercancel')],
    [button, touchEvent('lostpointercapture')],
    [window, new window.Event('scroll')],
    [window, new window.Event('blur')],
    [window, touchEvent('pointerdown', { pointerId: 2, isPrimary: false })],
  ]) {
    await act(async () => button.dispatchEvent(touchEvent('pointerdown')))
    assert.equal(clock.pending.size, 1)
    await act(async () => target.dispatchEvent(event))
    assert.equal(clock.pending.size, 0, event.type)
    await clock.finish()
    assert.equal(document.querySelector('[role="menu"]'), null, event.type)
  }
})

test('native touch contextmenu cancels the hold timer and suppresses the following click', async (t) => {
  await render(t)
  const clock = holdClock(t)
  const button = document.querySelector('[data-file-id]')
  await act(async () => button.dispatchEvent(touchEvent('pointerdown')))
  await openMenu()
  assert.equal(clock.pending.size, 0)
  await act(async () => button.click())
  assert.equal(button.getAttribute('aria-pressed'), 'true')
  assert.ok(document.querySelector('[role="menu"]'))
})

test('file action toolbar supports keyboard navigation and skips disabled commands', async (t) => {
  await render(t, [file, { ...file, id: 'second-file', name: 'second.txt' }])
  const files = [...document.querySelectorAll('[data-file-id]')]
  await act(async () => {
    files[0].click()
    files[1].dispatchEvent(new window.MouseEvent('click', { bubbles: true, ctrlKey: true }))
  })
  const toolbar = document.querySelector('[role="toolbar"]')
  assert.equal(toolbar.getAttribute('aria-label'), 'Действия с выбранными файлами')
  const download = toolbar.querySelector('[aria-label="Скачать"]')
  const remove = toolbar.querySelector('[aria-label="Удалить"]')
  download.focus()
  for (const [key, expected] of [['ArrowRight', remove], ['ArrowRight', download], ['End', remove], ['Home', download]]) {
    await act(async () => document.activeElement.dispatchEvent(new window.KeyboardEvent('keydown', { key, bubbles: true })))
    assert.equal(document.activeElement, expected)
  }
})

test('sticky toolbar preserves search and shows selected and total counters in a separate page footer', async (t) => {
  const { default: FileUpload } = require('../src/features/files/FileUpload.tsx')
  const { loadFiles } = require('../src/features/files/filesSlice.ts')
  const originalObserver = globalThis.IntersectionObserver
  let intersection
  globalThis.IntersectionObserver = class {
    constructor(callback) { intersection = callback }
    observe() {}
    disconnect() {}
  }
  t.after(() => { globalThis.IntersectionObserver = originalObserver })
  const store = configureStore({ reducer: {
    files: filesReducer, users: usersReducer, permissions: permissionsReducer,
    auth: () => ({ token: 'token' }),
  } })
  store.dispatch(loadFiles.fulfilled([file, { ...file, id: 'second-file', name: 'second.txt' }], 'loaded', file.owner_id))
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  t.after(async () => { await act(async () => root.unmount()); host.remove() })
  await act(async () => root.render(React.createElement(Provider, { store }, React.createElement(FileUpload, { ownerId: file.owner_id }))))
  const toolbar = document.querySelector('.file-selection-toolbar')
  const count = document.querySelector('.file-page-counters')
  assert.equal(count.getAttribute('aria-hidden'), 'true')
  assert.equal(count.hasAttribute('inert'), true)
  assert.equal(toolbar.querySelector('[aria-label="Вид файлов"]') !== null, true)
  await act(async () => toolbar.querySelector('input[value="grid"]').click())
  assert.equal(store.getState().files.viewMode, 'grid')
  await act(async () => toolbar.querySelector('[aria-label="Открыть поиск файлов"]').click())
  const input = toolbar.querySelector('input[type="search"]')
  await act(async () => {
    Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(input, 'no-match')
    input.dispatchEvent(new window.Event('input', { bubbles: true }))
  })
  assert.equal(document.activeElement === input, true, 'filtering to zero results preserves search focus')
  assert.equal(document.querySelectorAll('[data-file-id]').length, 0)
  assert.equal(input.isConnected, true)
  await act(async () => intersection([{ isIntersecting: false, boundingClientRect: { top: -1 } }]))
  assert.equal(toolbar.classList.contains('file-selection-toolbar--stuck'), true)
  assert.equal(count.getAttribute('aria-hidden'), 'false')
  assert.equal(count.dataset.visible, 'true')
  assert.deepEqual([...count.querySelectorAll('span')].map((span) => span.textContent), ['Выбрано: 0', 'Всего файлов: 2'])
  assert.equal(toolbar.contains(count), false)
  assert.equal(count.parentElement === document.body, true)
  await act(async () => toolbar.querySelector('[aria-label="Закрыть поиск"]').click())
  assert.equal(document.querySelectorAll('[data-file-id]').length, 2)
  await act(async () => document.querySelector('[data-file-id]').click())
  assert.deepEqual([...count.querySelectorAll('span')].map((span) => span.textContent), ['Выбрано: 1', 'Всего файлов: 2'])
  await act(async () => intersection([{ isIntersecting: true, boundingClientRect: { top: 1 } }]))
  assert.equal(count.getAttribute('aria-hidden'), 'true')
  assert.equal(count.dataset.visible, 'false')
  assert.equal(count.isConnected, true, 'panel stays mounted so its exit transition can finish')
})

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
  const store = await render(t)
  await openMenu()
  await activate('Удалить')
  assert.equal(remove.mock.callCount(), 0)
  assert.ok(document.querySelector('.p-desktop-modal[open]'))
  await act(async () => document.querySelector('#file-delete-dialog .p-desktop-modal__action--destructive').click())
  assert.equal(remove.mock.callCount(), 1)
  assert.deepEqual(remove.mock.calls[0].arguments, [file.id, 'token'])
  assert.deepEqual(store.getState().files.ids, [])
})

test('file falls only after server deletion succeeds and is removed after the animation', async (t) => {
  let acknowledge
  t.mock.method(actions, 'deleteStoredFile', () => new Promise((resolve) => { acknowledge = resolve }))
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
  await act(async () => document.querySelector('#file-delete-dialog .p-desktop-modal__action--destructive').click())
  await act(async () => {
    const event = new window.Event('animationend', { bubbles: true })
    Object.defineProperty(event, 'animationName', { value: 'delete-dialog-exit' })
    document.querySelector('#file-delete-dialog').dispatchEvent(event)
  })
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

test('delete confirmation focuses Cancel and cancels without deleting on button or Escape', async (t) => {
  const remove = t.mock.method(actions, 'deleteStoredFile', async () => {})
  const store = await render(t)
  for (const method of ['button', 'escape']) {
    await openMenu()
    await activate('Удалить')
    const dialog = document.querySelector('.p-desktop-modal[open]')
    const cancel = dialog.querySelector('.p-desktop-modal__action--secondary')
    assert.equal(document.activeElement, cancel)
    assert.ok(dialog.textContent.includes(file.name))
    await act(async () => {
      if (method === 'button') cancel.click()
      else dialog.dispatchEvent(new window.Event('cancel', { cancelable: true }))
    })
    assert.equal(document.querySelector('#file-delete-dialog'), null)
    assert.equal(remove.mock.callCount(), 0)
    assert.deepEqual(store.getState().files.ids, [file.id])
    assert.equal(document.querySelector('[data-file-id]').getAttribute('aria-pressed'), 'true')
  }
})

test('delete modal keeps Puppertino typography under p-type and resets native dialog chrome', async (t) => {
  const path = require('node:path')
  const typography = fs.readFileSync(path.join(__dirname, '../node_modules/@codedgar/puppertino/dist/css/typography.css'), 'utf8')
  const modals = fs.readFileSync(path.join(__dirname, '../node_modules/@codedgar/puppertino/dist/css/modals.css'), 'utf8')
  const application = fs.readFileSync(path.join(__dirname, '../src/styles.css'), 'utf8')
  function rule(source, selector) {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const match = source.match(new RegExp(`(?:^|\\n)${escaped} \\{[^}]+\\}`))
    assert.ok(match, `missing CSS rule: ${selector}`)
    return match[0]
  }
  const style = document.createElement('style')
  style.textContent = [
    rule(typography, '.p-type h2'), rule(typography, '.p-type p'),
    rule(modals, '.p-desktop-modal__title'), rule(modals, '.p-desktop-modal__description'),
    rule(application, '.file-delete-dialog'), rule(application, '.file-delete-dialog .p-desktop-modal__actions'),
  ].join('\n')
  document.head.append(style)
  document.body.classList.add('p-type')
  t.after(() => { style.remove(); document.body.classList.remove('p-type') })
  await render(t)
  await openMenu()
  await activate('Удалить')
  const modal = document.querySelector('#file-delete-dialog')
  assert.equal(window.getComputedStyle(modal.querySelector('[role="heading"]')).fontSize, '13px')
  assert.equal(window.getComputedStyle(modal.querySelector('#delete-description')).fontSize, '11px')
  assert.equal(window.getComputedStyle(modal).padding, '0px')
  assert.equal(window.getComputedStyle(modal).margin, '0px')
  assert.equal(window.getComputedStyle(modal).borderTopWidth, '0px')
  assert.equal(window.getComputedStyle(modal.querySelector('.p-desktop-modal__actions')).gap, '8px')
})

test('delete modal touches its live anchor, follows scrolling and closes after its exit animation', async (t) => {
  const { default: DeleteConfirmation } = require('../src/features/files/DeleteConfirmation.tsx')
  t.mock.method(window, 'matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
  const source = document.createElement('button')
  const host = document.createElement('div')
  document.body.append(source, host)
  let sourceTop = 200
  source.getBoundingClientRect = () => ({ left: 600, right: 700, top: sourceTop, bottom: sourceTop + 44, width: 100, height: 44 })
  const root = createRoot(host)
  const cancelled = t.mock.fn()
  const confirmed = t.mock.fn()
  t.after(async () => { await act(async () => root.unmount()); source.remove(); host.remove() })
  await act(async () => root.render(React.createElement(DeleteConfirmation, {
    files: [file], anchor: { element: source }, onCancel: cancelled, onConfirm: confirmed,
  })))
  const modal = document.querySelector('#file-delete-dialog')
  Object.defineProperty(modal, 'offsetWidth', { value: 420 })
  Object.defineProperty(modal, 'offsetHeight', { value: 160 })
  await act(async () => window.dispatchEvent(new window.Event('resize')))
  assert.equal(modal.style.left, '280px')
  assert.equal(modal.style.top, '244px')
  sourceTop = 100
  await act(async () => window.dispatchEvent(new window.Event('scroll')))
  assert.equal(modal.style.top, '144px')
  sourceTop = 700
  await act(async () => window.dispatchEvent(new window.Event('scroll')))
  assert.equal(modal.style.top, '540px', 'flips above the button without a gap near the bottom edge')
  await act(async () => modal.querySelector('.p-desktop-modal__action--secondary').click())
  assert.equal(modal.dataset.closing, 'true')
  assert.ok(modal.open)
  assert.equal(cancelled.mock.callCount(), 0)
  const end = (name) => {
    const event = new window.Event('animationend', { bubbles: true })
    Object.defineProperty(event, 'animationName', { value: name })
    modal.dispatchEvent(event)
  }
  await act(async () => end('delete-dialog-enter'))
  assert.ok(modal.open)
  await act(async () => end('delete-dialog-exit'))
  assert.equal(modal.open, false)
  assert.equal(cancelled.mock.callCount(), 1)
  assert.equal(confirmed.mock.callCount(), 0)
})

test('context menu opens permissions despite a blur before click', async (t) => {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  const load = t.mock.method(permissionsApi, 'loadFilePermissions', async () => [])
  await render(t)
  await openMenu()
  await activate('Права доступа')
  assert.ok(document.querySelector('dialog[open]'))
  assert.equal(document.querySelector('dialog h2').textContent, file.name)
  assert.equal(load.mock.callCount(), 1)
})

async function permissionsWindow(t, permissions = []) {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  t.mock.method(permissionsApi, 'loadFilePermissions', async () => permissions)
  const store = await render(t)
  await openMenu()
  await activate('Права доступа')
  return store
}

async function enterPermissionUser(value) {
  const input = document.querySelector('#permissions-user')
  await act(async () => {
    Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(input, value)
    input.dispatchEvent(new window.Event('input', { bubbles: true }))
  })
}

test('permissions opens with an owner summary and adds a user with read access by default', async (t) => {
  const save = t.mock.method(permissionsApi, 'saveFilePermissions', async () => {})
  const store = await permissionsWindow(t)
  assert.equal(document.querySelector('#permissions-editor-form'), null)
  assert.match(document.querySelector('.permissions-owner').textContent, /Owner \(вы\).*Полный доступ/)
  assert.equal(document.querySelector('.permissions-owner button'), null)
  const add = document.querySelector('[aria-controls="permissions-editor-form"]')
  await act(async () => add.click())
  assert.equal(document.activeElement, document.querySelector('#permissions-user'))
  const id = 'aabbccdd-1111-2222-3333-444455556666'
  await enterPermissionUser(id)
  const boxes = [...document.querySelectorAll('.permissions-choices input')]
  assert.deepEqual(boxes.map((box) => box.checked), [true, false, false])
  await act(async () => document.querySelector('#permissions-editor-form').dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })))
  assert.deepEqual(save.mock.calls[0].arguments, [file.id, id, ['files:read'], 'token'])
  assert.equal(document.querySelector('#permissions-editor-form'), null)
  assert.equal(document.activeElement, add)
  assert.equal(store.getState().permissions.byFileId[file.id][0].user_id, id)
  assert.match(document.querySelector('.permissions-users').textContent, new RegExp(id))
})

test('editing permissions preserves the user and changes, including after a failed save', async (t) => {
  const id = 'aabbccdd-1111-2222-3333-444455556666'
  const save = t.mock.method(permissionsApi, 'saveFilePermissions', async () => { throw new Error('Сервер недоступен') })
  await permissionsWindow(t, [{ file_id: file.id, user_id: id, code: 'files:read' }])
  await act(async () => document.querySelector(`[aria-label="Изменить права ${id}"]`).click())
  assert.equal(document.querySelector('#permissions-user').readOnly, true)
  assert.equal(document.querySelector('#permissions-user').value, id)
  const boxes = [...document.querySelectorAll('.permissions-choices input')]
  await act(async () => { boxes[0].click(); boxes[2].click() })
  await act(async () => document.querySelector('#permissions-editor-form').dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })))
  assert.deepEqual(save.mock.calls[0].arguments, [file.id, id, ['files:delete'], 'token'])
  assert.ok(document.querySelector('#permissions-editor-form'))
  assert.deepEqual(boxes.map((box) => box.checked), [false, false, true])
  assert.match(document.querySelector('#permissions-editor-form').textContent, /Сервер недоступен/)
  await act(async () => document.querySelector('.permissions-editor-actions button[type="button"]').click())
  assert.equal(document.querySelector('#permissions-editor-form'), null)
  assert.equal(save.mock.callCount(), 1)
})

test('removing all permissions makes revocation explicit and keeps the owner protected', async (t) => {
  const id = 'aabbccdd-1111-2222-3333-444455556666'
  const save = t.mock.method(permissionsApi, 'saveFilePermissions', async () => {})
  await permissionsWindow(t, [
    { file_id: file.id, user_id: file.owner_id, code: 'files:read' },
    { file_id: file.id, user_id: id, code: 'files:read' },
  ])
  assert.equal(document.querySelectorAll('.permissions-users li').length, 2, 'owner is listed exactly once')
  await act(async () => document.querySelector(`[aria-label="Изменить права ${id}"]`).click())
  await act(async () => document.querySelector('.permissions-choices input').click())
  const submit = document.querySelector('.permissions-editor-actions button[type="submit"]')
  assert.equal(submit.textContent, 'Отозвать доступ')
  assert.ok(submit.classList.contains('p-button-destructive'))
  await act(async () => document.querySelector('#permissions-editor-form').dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })))
  assert.deepEqual(save.mock.calls[0].arguments, [file.id, id, [], 'token'])
  assert.equal(document.querySelectorAll('.permissions-users li').length, 1)
})

test('permissions load failure offers retry without enabling access changes', async (t) => {
  t.mock.method(actions, 'copyStoredFilePath', async () => {})
  let attempts = 0
  t.mock.method(permissionsApi, 'loadFilePermissions', async () => {
    if (++attempts === 1) throw new Error('Не удалось загрузить права')
    return []
  })
  await render(t)
  await openMenu()
  await activate('Права доступа')
  const add = document.querySelector('[aria-controls="permissions-editor-form"]')
  assert.equal(add.disabled, true)
  const retry = [...document.querySelectorAll('dialog button')].find((button) => button.textContent === 'Повторить загрузку')
  await act(async () => retry.click())
  assert.equal(add.disabled, false)
  assert.equal(attempts, 2)
})

test('opening access does not copy or focus the URL; the copy control performs the explicit action', async (t) => {
  const copy = t.mock.method(actions, 'copyStoredFilePath', async () => {})
  t.mock.method(permissionsApi, 'loadFilePermissions', async () => [])
  await render(t)
  await openMenu()
  await activate('Права доступа')
  assert.equal(copy.mock.callCount(), 0)
  const close = document.querySelector('[aria-label="Закрыть окно общего доступа"]')
  assert.equal(document.activeElement === close, true, 'focus starts at the close control')
  assert.match(document.querySelector('#share-copy-status').textContent, /только пользователи с доступом/)
  await act(async () => document.querySelector('[aria-label="Скопировать ссылку"]').click())
  assert.equal(copy.mock.callCount(), 1)
  assert.equal(copy.mock.calls[0].arguments[0], file.id)
  assert.equal(copy.mock.calls[0].arguments[1] === document.querySelector('#share-file-url'), true)
  assert.match(document.querySelector('#share-copy-status').textContent, /Ссылка скопирована/)
})

test('closing a dirty permissions draft offers continuation and never saves implicitly', async (t) => {
  const save = t.mock.method(permissionsApi, 'saveFilePermissions', async () => {})
  await permissionsWindow(t)
  await act(async () => document.querySelector('[aria-controls="permissions-editor-form"]').click())
  await enterPermissionUser('aabbccdd-1111-2222-3333-444455556666')
  const dialog = document.querySelector('dialog')
  await act(async () => dialog.dispatchEvent(new window.Event('cancel', { cancelable: true })))
  assert.ok(document.querySelector('.permissions-discard'))
  assert.equal(document.activeElement.textContent, 'Продолжить редактирование')
  assert.ok(document.querySelector('.permissions-dialog-body').hasAttribute('inert'))
  await act(async () => document.activeElement.click())
  assert.equal(document.querySelector('.permissions-discard'), null)
  assert.equal(document.querySelector('#permissions-user').value, 'aabbccdd-1111-2222-3333-444455556666')
  await act(async () => document.querySelector('[aria-label="Закрыть окно общего доступа"]').click())
  const discard = [...document.querySelectorAll('.permissions-discard button')].find((button) => button.textContent === 'Отменить изменения')
  await act(async () => discard.click())
  assert.equal(document.querySelector('dialog'), null)
  assert.equal(save.mock.callCount(), 0)
})

test('permissions sheet follows the visible viewport when a software keyboard opens', async (t) => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  const viewport = Object.assign(new window.EventTarget(), { height: 480, offsetTop: 0 })
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: viewport })
  t.after(() => {
    if (descriptor) Object.defineProperty(window, 'visualViewport', descriptor)
    else delete window.visualViewport
  })
  await permissionsWindow(t)
  const dialog = document.querySelector('dialog')
  assert.equal(dialog.style.getPropertyValue('--permissions-viewport-height'), '480px')
  assert.equal(dialog.style.getPropertyValue('--permissions-keyboard-offset'), `${window.innerHeight - 480}px`)
  viewport.height = 600
  await act(async () => viewport.dispatchEvent(new window.Event('resize')))
  assert.equal(dialog.style.getPropertyValue('--permissions-viewport-height'), '600px')
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
