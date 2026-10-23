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
const { default: filesReducer, uploadFile, loadFiles } = require('../src/features/files/filesSlice.ts')
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
