const assert = require('node:assert/strict')
const fs = require('node:fs')
const test = require('node:test')
const ts = require('typescript')
const { configureStore } = require('@reduxjs/toolkit')

// Load the application's TypeScript in Node without a second test toolchain.
require.extensions['.ts'] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  module._compile(outputText, filename)
}

const { apiBlobRequest, serializeApiError } = require('../src/api/client.ts')
const { openFile } = require('../src/features/files/openFile.ts')
const { deleteStoredFile, downloadStoredFile } = require('../src/features/files/fileActions.ts')
const { selectFileIds, intersects } = require('../src/features/files/fileSelection.ts')
const { default: filesReducer, uploadFile, loadFiles } = require('../src/features/files/filesSlice.ts')
const { createUploadQueue } = require('../src/features/files/uploadQueue.ts')
const { listenForPageFileDrops } = require('../src/features/files/pageFileDrop.ts')
const storedFile = { id: 'file-id', name: 'sample.txt', type: 'text/plain', owner_id: 'owner', status: 'ready', size: 10 }
const json = (body, status = 200) => Response.json(body, { status })
const makeStore = () => configureStore({
  reducer: { files: filesReducer, auth: () => ({ token: 'test-token' }) },
})
const mockWindow = (t, value) => {
  const previous = globalThis.window
  globalThis.window = value
  t.after(() => {
    if (previous === undefined) delete globalThis.window
    else globalThis.window = previous
  })
}

// Exercise chunk requests through the same mocked server as metadata requests.
globalThis.XMLHttpRequest = class {
  upload = {}
  headers = new Headers()
  controller = new AbortController()
  open(method, path) { this.method = method; this.path = path }
  setRequestHeader(name, value) { this.headers.set(name, value) }
  abort() { this.controller.abort() }
  async send(body) {
    try {
      const response = await fetch(this.path, {
        method: this.method, headers: this.headers, body, signal: this.controller.signal,
      })
      this.status = response.status
      this.responseText = await response.text()
      if (response.ok) this.upload.onprogress?.({ loaded: body.size })
      this.onload?.()
    } catch (error) {
      if (this.controller.signal.aborted) this.onabort?.()
      else this.onerror?.(error)
    } finally {
      this.onloadend?.()
    }
  }
}

function dragEvent(type, files = [], types = ['Files']) {
  return Object.assign(new Event(type, { cancelable: true }), {
    dataTransfer: { files, types, dropEffect: 'none' },
  })
}

test('selects a single file, toggles individual files and extends ranges in visible order', () => {
  const ids = ['a', 'b', 'c', 'd']
  assert.deepEqual(selectFileIds(['a'], ids, 'c', 'a', { toggle: false, range: false }), ['c'])
  assert.deepEqual(selectFileIds(['a'], ids, 'c', 'a', { toggle: true, range: false }), ['a', 'c'])
  assert.deepEqual(selectFileIds(['a', 'c'], ids, 'a', 'c', { toggle: true, range: false }), ['c'])
  assert.deepEqual(selectFileIds(['d'], ids, 'a', 'c', { toggle: false, range: true }), ['a', 'b', 'c'])
  assert.deepEqual(selectFileIds(['d'], ids, 'c', 'a', { toggle: true, range: true }), ['d', 'a', 'b', 'c'])
  assert.deepEqual(selectFileIds(['d'], ids, 'b', 'hidden', { toggle: false, range: true }), ['b'])
  assert.deepEqual(selectFileIds(['d'], ids, 'hidden', 'a', { toggle: false, range: true }), ['d'])
})

test('marquee includes partially overlapping files but excludes files outside its bounds', () => {
  const box = { left: 10, top: 10, right: 100, bottom: 100 }
  assert.equal(intersects(box, { left: 90, top: 90, right: 150, bottom: 150 }), true)
  assert.equal(intersects(box, { left: 20, top: 20, right: 40, bottom: 40 }), true)
  assert.equal(intersects(box, { left: 100, top: 10, right: 150, bottom: 100 }), false)
  assert.equal(intersects(box, { left: 10, top: 101, right: 100, bottom: 150 }), false)
})

test('deletes files with authentication and reports rejected deletions', async (t) => {
  const originalFetch = globalThis.fetch
  t.after(() => { globalThis.fetch = originalFetch })
  globalThis.fetch = async (path, options) => {
    assert.equal(path, '/v1/files/file%2Fid')
    assert.equal(options.method, 'DELETE')
    assert.equal(options.headers.get('Authorization'), 'Bearer token')
    return new Response(null, { status: 204 })
  }
  await deleteStoredFile('file/id', 'token')
  globalThis.fetch = async () => json({ error: 'Нет доступа' }, 403)
  await assert.rejects(deleteStoredFile('file/id', 'token'), /Нет доступа/)
})

test('downloads authenticated content with its filename and releases the temporary URL', async (t) => {
  const previous = { fetch: globalThis.fetch, document: globalThis.document, create: URL.createObjectURL, revoke: URL.revokeObjectURL }
  t.after(() => {
    globalThis.fetch = previous.fetch
    if (previous.document === undefined) delete globalThis.document
    else globalThis.document = previous.document
    URL.createObjectURL = previous.create
    URL.revokeObjectURL = previous.revoke
  })
  let clicked = false
  let removed = false
  let cleanup
  const revoked = []
  const link = { click() { clicked = true }, remove() { removed = true } }
  globalThis.document = { createElement: () => link, body: { append() {} } }
  mockWindow(t, { setTimeout(callback) { cleanup = callback } })
  URL.createObjectURL = (blob) => { assert.equal(blob.size, 7); return 'blob:download' }
  URL.revokeObjectURL = (url) => revoked.push(url)
  globalThis.fetch = async (path, options) => {
    assert.equal(path, '/v1/files/file%2Fid/content')
    assert.equal(options.headers.get('Authorization'), 'Bearer token')
    return new Response('content')
  }
  await downloadStoredFile('file/id', 'Отчёт.txt', 'token')
  assert.equal(link.download, 'Отчёт.txt')
  assert.equal(link.href, 'blob:download')
  assert.ok(clicked && removed)
  assert.deepEqual(revoked, [])
  cleanup()
  assert.deepEqual(revoked, ['blob:download'])
  clicked = false
  globalThis.fetch = async () => json({ error: 'Нет доступа' }, 403)
  await assert.rejects(downloadStoredFile('file/id', 'Отчёт.txt', 'token'), /Нет доступа/)
  assert.equal(clicked, false)
})

test('accepts multiple files dropped on the page and prevents browser navigation', () => {
  const page = new EventTarget()
  const received = []
  const dragging = []
  const stop = listenForPageFileDrops(page, (files) => received.push(files), (value) => dragging.push(value))
  const files = [new File(['one'], 'one.txt'), new File(['two'], 'two.txt')]
  const enter = dragEvent('dragenter')
  page.dispatchEvent(enter)
  assert.ok(enter.defaultPrevented)
  const over = dragEvent('dragover')
  page.dispatchEvent(over)
  assert.ok(over.defaultPrevented)
  assert.equal(over.dataTransfer.dropEffect, 'copy')
  const drop = dragEvent('drop', files)
  page.dispatchEvent(drop)
  assert.ok(drop.defaultPrevented)
  assert.deepEqual(received, [files])
  assert.deepEqual(dragging, [true, false])
  stop()
})

test('keeps the drop hint visible across nested elements and clears it on exit or cancellation', () => {
  const page = new EventTarget()
  let dragging = false
  const stop = listenForPageFileDrops(page, () => assert.fail('Unexpected upload'), (value) => { dragging = value })
  page.dispatchEvent(dragEvent('dragenter'))
  page.dispatchEvent(dragEvent('dragenter'))
  page.dispatchEvent(dragEvent('dragleave'))
  assert.ok(dragging)
  page.dispatchEvent(dragEvent('dragleave'))
  assert.equal(dragging, false)
  for (const type of ['dragend', 'blur']) {
    page.dispatchEvent(dragEvent('dragenter'))
    page.dispatchEvent(new Event(type))
    assert.equal(dragging, false)
  }
  stop()
})

test('ignores text drags and removes page listeners on cleanup', () => {
  const page = new EventTarget()
  const received = []
  const stop = listenForPageFileDrops(page, (files) => received.push(files), () => {})
  for (const type of ['dragenter', 'dragover', 'drop']) {
    const event = dragEvent(type, [], ['text/plain'])
    page.dispatchEvent(event)
    assert.equal(event.defaultPrevented, false)
  }
  page.dispatchEvent(dragEvent('drop'))
  stop()
  const event = dragEvent('drop', [new File([], 'after-unmount')])
  page.dispatchEvent(event)
  assert.equal(event.defaultPrevented, false)
  assert.deepEqual(received, [])
})

test('hides the drop hint immediately on every window edge despite nested drag events', () => {
  const page = Object.assign(new EventTarget(), { innerWidth: 1000, innerHeight: 800 })
  let dragging = false
  const stop = listenForPageFileDrops(page, () => assert.fail('Unexpected upload'), (value) => { dragging = value })
  for (const [clientX, clientY] of [[0, 100], [100, 0], [1000, 100], [100, 800]]) {
    page.dispatchEvent(dragEvent('dragenter'))
    page.dispatchEvent(dragEvent('dragenter'))
    page.dispatchEvent(dragEvent('dragenter'))
    const leave = Object.assign(dragEvent('dragleave'), { clientX, clientY })
    page.dispatchEvent(leave)
    assert.equal(dragging, false)
    page.dispatchEvent(dragEvent('dragenter'))
    assert.equal(dragging, true)
    page.dispatchEvent(dragEvent('dragleave'))
    assert.equal(dragging, false)
  }
  stop()
})

test('uploads a large selection without limiting the number of files or overlapping uploads', async () => {
  const files = Array.from({ length: 250 }, (_, index) => new File([String(index)], 'same-name.txt'))
  const uploaded = []
  const finished = Promise.withResolvers()
  let active = 0
  const queue = createUploadQueue(async (file) => {
    active += 1
    assert.equal(active, 1)
    await Promise.resolve()
    uploaded.push(file)
    active -= 1
  }, (state) => {
    if (state.pending === 0) finished.resolve(state)
  })
  queue.add(files)
  const state = await finished.promise
  assert.deepEqual(uploaded, files)
  assert.deepEqual(state, { total: 250, completed: 250, failed: 0, pending: 0, errors: [] })
})

test('appends new selections while an upload is in progress', async () => {
  const first = Promise.withResolvers()
  const finished = Promise.withResolvers()
  const uploaded = []
  const queue = createUploadQueue(async (file) => {
    uploaded.push(file.name)
    if (file.name === 'first') await first.promise
  }, (state) => {
    if (state.pending === 0) finished.resolve(state)
  })
  queue.add([new File([], 'first'), new File([], 'second')])
  queue.add([new File([], 'third')])
  assert.deepEqual(uploaded, ['first'])
  first.resolve()
  const state = await finished.promise
  assert.deepEqual(uploaded, ['first', 'second', 'third'])
  assert.equal(state.completed, 3)
  assert.equal(state.total, 3)
})

test('reports failed filenames and continues uploading the remaining files', async () => {
  const finished = Promise.withResolvers()
  const uploaded = []
  const queue = createUploadQueue(async (file) => {
    if (file.name === 'bad') throw new Error('Disk full')
    uploaded.push(file.name)
  }, (state) => {
    if (state.pending === 0) finished.resolve(state)
  })
  queue.add([new File([], 'before'), new File([], 'bad'), new File([], 'after')])
  const state = await finished.promise
  assert.deepEqual(uploaded, ['before', 'after'])
  assert.equal(state.completed, 2)
  assert.equal(state.failed, 1)
  assert.deepEqual(state.errors, [{ filename: 'bad', message: 'Disk full' }])
})

test('ignores empty selections and starts fresh after a completed queue', async () => {
  const snapshots = []
  const queue = createUploadQueue(async () => {}, (state) => snapshots.push(state))
  queue.add([])
  assert.equal(snapshots.length, 0)
  queue.add([new File([], 'first')])
  await new Promise(setImmediate)
  queue.add([new File([], 'second')])
  await new Promise(setImmediate)
  assert.deepEqual(snapshots.at(-1), { total: 1, completed: 1, failed: 0, pending: 0, errors: [] })
})

test('aborts the active upload and discards waiting files when stopped', async () => {
  const active = Promise.withResolvers()
  const snapshots = []
  const uploaded = []
  let signal
  const queue = createUploadQueue(async (file, uploadSignal) => {
    uploaded.push(file.name)
    signal = uploadSignal
    await active.promise
  }, (state) => snapshots.push(state))
  queue.add([new File([], 'first'), new File([], 'second')])
  queue.stop()
  assert.ok(signal.aborted)
  active.resolve()
  await new Promise(setImmediate)
  queue.add([new File([], 'third')])
  assert.deepEqual(uploaded, ['first'])
  assert.equal(snapshots.length, 1)
})

test('uploads chunks at their original positions, then adds the ready file to the list', async (t) => {
  const parts = new Map()
  let completed = false
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    assert.equal(options.headers.get('Authorization'), 'Bearer test-token')
    if (path === '/v1/files/upload') {
      assert.deepEqual(JSON.parse(options.body), {
        owner_id: 'owner', name: 'sample.txt', type: 'text/plain', size: 10,
      })
      return json({ file: { ...storedFile, status: 'uploading' }, part_size: 3 })
    }
    const part = path.match(/\/parts\/(\d+)$/)
    if (part) {
      parts.set(Number(part[1]), await options.body.text())
      return json({ part: {} })
    }
    assert.equal(path, '/v1/uploads/file-id/complete')
    assert.equal(parts.size, 4)
    completed = true
    return json({ file: storedFile })
  })
  const store = makeStore()
  const result = await store.dispatch(uploadFile({
    file: new File(['0123456789'], 'sample.txt', { type: 'text/plain' }), ownerId: 'owner',
  }))
  assert.ok(uploadFile.fulfilled.match(result))
  assert.ok(completed)
  assert.equal([...parts].sort(([a], [b]) => a - b).map(([, data]) => data).join(''), '0123456789')
  assert.deepEqual(store.getState().files.entities['file-id'], storedFile)
  assert.equal(store.getState().files.upload.uploadedBytes, 10)
})

test('completes an empty upload without sending empty parts', async (t) => {
  const paths = []
  t.mock.method(globalThis, 'fetch', async (path) => {
    paths.push(path)
    return path === '/v1/files/upload'
      ? json({ file: storedFile, part_size: 3 })
      : json({ file: { ...storedFile, size: 0 } })
  })
  const result = await makeStore().dispatch(uploadFile({ file: new File([], 'empty.txt'), ownerId: 'owner' }))
  assert.ok(uploadFile.fulfilled.match(result))
  assert.deepEqual(paths, ['/v1/files/upload', '/v1/uploads/file-id/complete'])
})

test('updates progress during parallel chunk transfers without double counting bytes', async (t) => {
  const requests = []
  const complete = Promise.withResolvers()
  t.mock.method(XMLHttpRequest.prototype, 'send', function (body) {
    this.body = body
    requests.push(this)
  })
  t.mock.method(globalThis, 'fetch', async (path) => {
    if (path === '/v1/files/upload') return json({ file: storedFile, part_size: 6 })
    assert.equal(path, '/v1/uploads/file-id/complete')
    await complete.promise
    return json({ file: storedFile })
  })
  const store = makeStore()
  const pending = store.dispatch(uploadFile({ file: new File(['0123456789'], 'photo.jpg'), ownerId: 'owner' }))
  await new Promise(setImmediate)
  assert.equal(requests.length, 2)
  requests[0].upload.onprogress({ loaded: 2 })
  requests[1].upload.onprogress({ loaded: 1 })
  assert.equal(store.getState().files.upload.uploadedBytes, 3)
  requests[0].upload.onprogress({ loaded: 2 })
  assert.equal(store.getState().files.upload.uploadedBytes, 3)
  for (const request of requests) {
    request.status = 204
    request.upload.onprogress({ loaded: request.body.size })
    request.onload()
    request.onloadend()
  }
  await new Promise(setImmediate)
  assert.equal(store.getState().files.upload.uploadedBytes, 10)
  assert.equal(store.getState().files.upload.status, 'uploading')
  assert.deepEqual(store.getState().files.ids, [])
  complete.resolve()
  assert.ok(uploadFile.fulfilled.match(await pending))
})

test('aborts an in-flight chunk request and reports cancellation', async (t) => {
  const { apiUploadPart } = require('../src/api/client.ts')
  let request
  t.mock.method(XMLHttpRequest.prototype, 'send', function () { request = this })
  t.mock.method(XMLHttpRequest.prototype, 'abort', function () {
    this.onabort()
    this.onloadend()
  })
  const controller = new AbortController()
  const pending = apiUploadPart('/part', new Blob(['photo']), controller.signal, () => {})
  assert.ok(request)
  controller.abort()
  await assert.rejects(pending, { name: 'AbortError' })
})

test('cancels a failed upload and keeps it out of the file list', async (t) => {
  let cancelled = false
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    if (path === '/v1/files/upload') return json({ file: storedFile, part_size: 3 })
    if (options.method === 'DELETE') {
      cancelled = true
      return new Response(null, { status: 204 })
    }
    assert.match(path, /\/parts\//)
    return json({ error: 'Disk full' }, 500)
  })
  const store = makeStore()
  const result = await store.dispatch(uploadFile({ file: new File(['data'], 'sample.txt'), ownerId: 'owner' }))
  assert.ok(uploadFile.rejected.match(result))
  assert.ok(cancelled)
  assert.deepEqual(store.getState().files.ids, [])
  assert.equal(store.getState().files.upload.error.message, 'Disk full')
})

test('loads the existing file list with authentication', async (t) => {
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    assert.equal(path, '/v1/users/owner/files')
    assert.equal(options.headers.get('Authorization'), 'Bearer test-token')
    return json({ files: [storedFile] })
  })
  const store = makeStore()
  await store.dispatch(loadFiles('owner'))
  assert.deepEqual(store.getState().files.entities['file-id'], storedFile)
})

test('reads binary content with authentication and preserves its MIME type', async (t) => {
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    assert.equal(path, '/v1/files/file-id/content')
    assert.equal(options.headers.get('Authorization'), 'Bearer test-token')
    return new Response(new Uint8Array([0, 255, 42]), { headers: { 'Content-Type': 'image/png' } })
  })
  const blob = await apiBlobRequest('/v1/files/file-id/content', {}, 'test-token')
  assert.equal(blob.type, 'image/png')
  assert.deepEqual([...new Uint8Array(await blob.arrayBuffer())], [0, 255, 42])
})

test('reports an API error instead of displaying it as file content', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => json({ error: 'File not found' }, 404))
  await assert.rejects(apiBlobRequest('/v1/files/missing/content'), (error) => {
    assert.deepEqual(serializeApiError(error), { message: 'File not found', status: 404, fields: undefined })
    return true
  })
})

test('opens raw video content in a new tab and releases its URL after closing', async (t) => {
  let location
  let checkClosed
  let cleared = false
  const tab = {
    opener: {}, closed: false,
    sessionStorage: { clear() { cleared = true } },
    location: { replace(url) { location = url } },
  }
  mockWindow(t, {
    open(path, target) {
      assert.equal(path, 'about:blank')
      assert.equal(target, '_blank')
      return tab
    },
    setInterval(callback) { checkClosed = callback; return 1 },
    clearInterval(timer) { assert.equal(timer, 1) },
  })
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    assert.equal(path, '/v1/files/video-id/content')
    assert.equal(options.headers.get('Authorization'), 'Bearer test-token')
    return new Response('video bytes', { headers: { 'Content-Type': 'video/quicktime' } })
  })
  t.mock.method(URL, 'createObjectURL', (blob) => {
    assert.equal(blob.type, 'video/quicktime')
    return 'blob:video'
  })
  const revoke = t.mock.method(URL, 'revokeObjectURL', () => {})
  await openFile('video-id', 'test-token')
  assert.equal(location, 'blob:video')
  assert.equal(tab.opener, null)
  assert.ok(cleared)
  checkClosed()
  assert.equal(revoke.mock.callCount(), 0)
  tab.closed = true
  checkClosed()
  assert.deepEqual(revoke.mock.calls[0].arguments, ['blob:video'])
})

test('closes the empty content tab when the request fails', async (t) => {
  let closed = false
  mockWindow(t, {
    open: () => ({ sessionStorage: { clear() {} }, close() { closed = true } }),
  })
  t.mock.method(globalThis, 'fetch', async () => json({ error: 'File not found' }, 404))
  await assert.rejects(openFile('missing', 'test-token'), /File not found/)
  assert.ok(closed)
})

test('reports a blocked tab without fetching the file', async (t) => {
  mockWindow(t, { open: () => null })
  const fetch = t.mock.method(globalThis, 'fetch', () => assert.fail('Unexpected fetch'))
  await assert.rejects(openFile('video-id', 'test-token'), /Браузер заблокировал/)
  assert.equal(fetch.mock.callCount(), 0)
})
