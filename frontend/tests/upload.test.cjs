const assert = require('node:assert/strict')
const fs = require('node:fs')
const test = require('node:test')
const ts = require('typescript')
const { configureStore } = require('@reduxjs/toolkit')
const installUploadXHR = require('./helpers/upload-xhr.cjs')

require.extensions['.ts'] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  module._compile(outputText, filename)
}
const { uploadStoredFile } = require('../src/features/files/uploadTransfer.ts')
const { default: filesReducer, uploadFile } = require('../src/features/files/filesSlice.ts')
const flush = () => new Promise(setImmediate)
const json = (body, status = 200) => Response.json(body, { status })

function server(t, hooks = {}, partSize = 4) {
  const state = { parts: new Map(), puts: [], cancelled: 0, completions: 0, ready: false, file: null, requests: installUploadXHR(t) }
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    assert.equal(options.headers.get('Authorization'), 'Bearer token')
    assert.equal(options.cache, 'no-store')
    if (path === '/v1/files/upload') {
      const input = JSON.parse(options.body)
      state.file = { ...input, id: 'file-id', status: 'uploading' }
      return hooks.init?.(state, options) ?? json({ file: state.file, part_size: partSize })
    }
    if (path.includes('/parts/')) {
      assert.ok(options.body instanceof ArrayBuffer, 'send a buffer, not a file-backed Blob')
      assert.equal(options.headers.get('Content-Type'), 'application/octet-stream')
      const position = Number(path.split('/').at(-1))
      state.puts.push(position)
      const save = () => {
        state.parts.set(position, Buffer.from(options.body))
        return json({ part: { position, size: options.body.byteLength } }, 201)
      }
      return hooks.put ? hooks.put({ state, position, options, save }) : save()
    }
    if (path.endsWith('/complete')) {
      state.completions++
      assert.equal(Buffer.concat([...state.parts.values()]).length, state.file.size)
      state.ready = true
      return hooks.complete?.(state, options) ?? json({ file: { ...state.file, status: 'ready' } })
    }
    if (path === '/v1/files/file-id') {
      return state.ready ? json({ file: { ...state.file, status: 'ready' } }) : json({ error: 'not ready' }, 404)
    }
    assert.equal(path, '/v1/uploads/file-id')
    if (options.method === 'DELETE') {
      state.cancelled++
      return hooks.cancel?.() ?? new Response(null, { status: 204 })
    }
    return hooks.status?.(state, options) ?? json({ upload: {
      file_id: 'file-id', size: state.file.size,
      parts: [...state.parts].map(([position, bytes]) => ({ position, size: bytes.length })),
    } })
  })
  return state
}

function upload(file, controller = new AbortController()) {
  const progress = [], phases = [], parts = []
  const promise = uploadStoredFile({
    file, ownerId: 'owner', token: 'token', signal: controller.signal,
    onProgress: (bytes) => progress.push(bytes),
    onPhase: (phase) => phases.push(phase),
    onPart: (current, total) => parts.push([current, total]),
  })
  return { promise, progress, phases, parts }
}

test('preserves bytes and order for empty, small, exact-boundary and multi-blob files', async (t) => {
  const limit = 16 * 1024 * 1024
  for (const size of [0, 17, limit, limit + 1, limit * 2 + 5]) {
    await t.test(`${size} bytes`, async (t) => {
      const state = server(t, {}, limit)
      const bytes = new Uint8Array(size)
      for (let index = 0; index < size; index++) bytes[index] = index % 251
      const current = upload(new File([bytes], 'mobile.zip'))
      const ready = await current.promise
      assert.equal(ready.status, 'ready')
      assert.deepEqual(Buffer.concat([...state.parts.values()]), Buffer.from(bytes))
      assert.deepEqual(state.puts, Array.from({ length: Math.ceil(size / limit) }, (_, index) => index))
      assert.equal(current.progress.at(-1), size)
      assert.equal(state.cancelled, 0)
      assert.equal(state.completions, 1)
    })
  }
})

test('sends one part at a time and only advances progress after confirmation', async (t) => {
  const gates = [Promise.withResolvers(), Promise.withResolvers()]
  const state = server(t, { put: async ({ position, save }) => { await gates[position].promise; return save() } })
  const current = upload(new File(['abcdefgh'], 'mobile.txt'))
  await flush()
  assert.deepEqual(state.puts, [0])
  assert.deepEqual(current.progress, [0])
  gates[0].resolve()
  await flush()
  assert.deepEqual(state.puts, [0, 1])
  assert.deepEqual(current.progress, [0, 4])
  gates[1].resolve()
  await current.promise
  assert.deepEqual(current.progress, [0, 4, 8])
})

test('reports browser upload progress without claiming that the part is stored', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const gate = Promise.withResolvers()
  const state = server(t, {
    put: async ({ save }) => { await gate.promise; return save() },
  })
  const transferred = [], confirmed = []
  const pending = uploadStoredFile({ file: new File(['abcd'], 'mobile.zip'), ownerId: 'owner', token: 'token',
    signal: new AbortController().signal, onPhase() {}, onPart() {},
    onProgress: (bytes) => confirmed.push(bytes), onTransferProgress: (bytes) => transferred.push(bytes),
  })
  await flush()
  for (const bytes of [1, 2, 3]) {
    state.requests[0].upload.onprogress({ loaded: bytes })
    assert.equal(transferred.at(-1), bytes)
    assert.deepEqual(confirmed, [0])
  }
  gate.resolve()
  await pending
  assert.deepEqual(confirmed, [0, 4])
})

test('a stored part completes through polling even when PUT ignores abort and never resolves', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  let putSignal
  const state = server(t, { put: ({ save, options }) => {
    save()
    putSignal = options.signal
    return new Promise(() => {})
  } })
  const current = upload(new File(['abcd'], 'iphone.zip'))
  await flush()
  assert.deepEqual(current.progress, [0])
  t.mock.timers.tick(2_000)
  assert.equal((await current.promise).status, 'ready')
  assert.equal(putSignal.aborted, true)
  assert.deepEqual(state.puts, [0])
  assert.deepEqual(current.progress, [0, 4])
  assert.equal(state.cancelled, 0)
})

test('does not acknowledge a truncated part or stale upload snapshot', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  let polls = 0
  const state = server(t, {
    put: ({ save }) => { save(); return new Promise(() => {}) },
    status: () => {
      polls++
      return json({ upload: { file_id: polls === 1 ? 'wrong-id' : 'file-id', size: 4,
        parts: [{ position: 0, size: polls === 2 ? 3 : 4 }],
      } })
    },
  })
  const current = upload(new File(['abcd'], 'iphone.zip'))
  for (let poll = 0; poll < 3; poll++) {
    await flush()
    assert.equal(state.completions, 0)
    assert.deepEqual(current.progress, [0])
    t.mock.timers.tick(2_000)
  }
  await current.promise
  assert.equal(polls, 3)
  assert.deepEqual(state.puts, [0])
})

test('retries failed transfers with identical bytes and exhausts retries before cleanup', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const bodies = []
  const state = server(t, { put: ({ options }) => {
    bodies.push(Buffer.from(options.body))
    throw new TypeError('Disconnected')
  } })
  const current = upload(new File(['abcd'], 'mobile.zip'))
  const rejected = assert.rejects(current.promise, /часть 1 из 1.*Ошибка сети/)
  await flush()
  t.mock.timers.tick(500)
  await flush()
  t.mock.timers.tick(1_000)
  await rejected
  assert.deepEqual(state.puts, [0, 0, 0])
  assert.ok(bodies.every((body) => body.equals(Buffer.from('abcd'))))
  assert.deepEqual(current.progress, [0])
  assert.equal(state.completions, 0)
  assert.equal(state.cancelled, 1)
})

test('permanent HTTP errors fail immediately and retain cleanup errors', async (t) => {
  const state = server(t, {
    put: () => json({ error: 'Forbidden' }, 403),
    cancel: () => json({ error: 'Cleanup unavailable' }, 503),
  })
  await assert.rejects(upload(new File(['abcd'], 'mobile.zip')).promise, /Forbidden.*Cleanup unavailable/)
  assert.deepEqual(state.puts, [0])
  assert.equal(state.completions, 0)
})

test('recovers a completion response lost after commit by reading the ready file', async (t) => {
  const state = server(t, { complete: () => { throw new TypeError('Response lost') } })
  assert.equal((await upload(new File(['abcd'], 'mobile.zip')).promise).status, 'ready')
  assert.equal(state.completions, 1)
  assert.equal(state.cancelled, 0)
})

test('a completion response that never settles is recovered after its deadline', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const state = server(t, { complete: () => new Promise(() => {}) })
  const current = upload(new File(['abcd'], 'mobile.zip'))
  await flush()
  t.mock.timers.tick(30_000)
  assert.equal((await current.promise).status, 'ready')
  assert.equal(state.completions, 1)
  assert.equal(state.cancelled, 0)
})

test('an unsaved part with no transport events eventually fails instead of polling forever', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const state = server(t, { put: () => new Promise(() => {}) })
  const current = upload(new File(['abcd'], 'mobile.zip'))
  const rejected = assert.rejects(current.promise, /часть 1 из 1.*Превышено время/)
  for (let attempt = 1; attempt <= 3; attempt++) {
    await flush()
    t.mock.timers.tick(600_000)
    await flush()
    if (attempt < 3) t.mock.timers.tick(500 * attempt)
  }
  await rejected
  assert.deepEqual(state.puts, [0, 0, 0])
  assert.equal(state.cancelled, 1)
  assert.equal(state.completions, 0)
})

test('a cleanup request that never settles cannot block the queue forever', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const state = server(t, { put: () => json({ error: 'Forbidden' }, 403), cancel: () => new Promise(() => {}) })
  const current = upload(new File(['abcd'], 'mobile.zip'))
  const rejected = assert.rejects(current.promise, /Forbidden.*Не удалось подтвердить отмену.*Превышено время/)
  await flush()
  t.mock.timers.tick(10_000)
  await rejected
  assert.equal(state.cancelled, 1)
})

test('retries completion only while the file remains unconfirmed', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const state = server(t, { complete: (state) => {
    if (state.completions < 3) { state.ready = false; return json({ error: 'Busy' }, 503) }
    return json({ file: { ...state.file, status: 'ready' } })
  } })
  const current = upload(new File(['abcd'], 'mobile.zip'))
  await flush()
  t.mock.timers.tick(500)
  await flush()
  t.mock.timers.tick(1_000)
  await current.promise
  assert.equal(state.completions, 3)
  assert.deepEqual(state.puts, [0])
})

test('cancelling a stalled transfer aborts the PUT, stops polling and cleans up', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  let putSignal, polls = 0
  const state = server(t, {
    put: ({ options }) => { putSignal = options.signal; return new Promise(() => {}) },
    status: () => { polls++; throw new TypeError('Offline') },
  })
  const controller = new AbortController()
  const current = upload(new File(['abcdefgh'], 'mobile.zip'), controller)
  const rejected = assert.rejects(current.promise)
  await flush()
  controller.abort()
  await rejected
  t.mock.timers.tick(60_000)
  await flush()
  assert.equal(putSignal.aborted, true)
  assert.equal(polls, 0)
  assert.deepEqual(state.puts, [0])
  assert.equal(state.cancelled, 1)
})

test('initialization and file-read deadlines release hung operations', async (t) => {
  for (const stage of ['init', 'read']) {
    await t.test(stage, async (t) => {
      t.mock.timers.enable({ apis: ['setTimeout'] })
      const state = server(t, stage === 'init' ? { init: () => new Promise(() => {}) } : {})
      const file = new File(['abcd'], 'mobile.zip')
      if (stage === 'read') file.slice = () => ({ arrayBuffer: () => new Promise(() => {}) })
      const current = upload(file)
      const rejected = assert.rejects(current.promise, /Превышено время/)
      await flush()
      t.mock.timers.tick(stage === 'init' ? 30_000 : 60_000)
      await rejected
      assert.equal(state.puts.length, 0)
      assert.equal(state.cancelled, stage === 'init' ? 0 : 1)
    })
  }
})

test('Redux publishes a ready file only after completion is confirmed', async (t) => {
  const complete = Promise.withResolvers()
  server(t, { complete: async (state) => { await complete.promise; return json({ file: { ...state.file, status: 'ready' } }) } })
  const store = configureStore({ reducer: { files: filesReducer, auth: () => ({ token: 'token' }) } })
  const pending = store.dispatch(uploadFile({ file: new File(['abcd'], 'mobile.zip'), ownerId: 'owner' }))
  await flush()
  assert.deepEqual(store.getState().files.ids, [])
  assert.equal(store.getState().files.upload.phase, 'finalizing')
  assert.equal(store.getState().files.upload.status, 'uploading')
  assert.equal(store.getState().files.upload.uploadedBytes, 4)
  complete.resolve()
  assert.ok(uploadFile.fulfilled.match(await pending))
  assert.deepEqual(store.getState().files.ids, ['file-id'])
  assert.equal(store.getState().files.upload.status, 'succeeded')
})
