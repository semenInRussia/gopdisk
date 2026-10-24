const assert = require('node:assert/strict')
const fs = require('node:fs')
const http = require('node:http')
const test = require('node:test')
const ts = require('typescript')
const installUploadXHR = require('./helpers/upload-xhr.cjs')

require.extensions['.ts'] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  module._compile(outputText, filename)
}
const { uploadStoredFile } = require('../src/features/files/uploadTransfer.ts')

test('real HTTP: a 23 MB file finishes when a stored 16 MiB PUT never gets a response', async (t) => {
  installUploadXHR(t)
  const partSize = 16 * 1024 * 1024
  const bytes = new Uint8Array(23_039_521)
  for (let index = 0; index < bytes.length; index++) bytes[index] = index % 251
  const parts = new Map()
  const putPositions = []
  let polls = 0, completions = 0
  const metadata = { id: 'http-file', name: 'iphone.zip', type: 'application/zip', owner_id: 'owner', size: bytes.length }
  const listener = http.createServer(async (request, response) => {
    try {
      assert.equal(request.headers.authorization, 'Bearer token')
      const reply = (body) => {
        response.setHeader('Content-Type', 'application/json')
        response.end(JSON.stringify(body))
      }
      if (request.url === '/v1/files/upload') {
        for await (const _chunk of request) { /* consume initialization */ }
        reply({ file: { ...metadata, status: 'uploading' }, part_size: partSize })
      } else if (request.method === 'PUT') {
        const position = Number(request.url.split('/').at(-1))
        const chunks = []
        for await (const chunk of request) chunks.push(chunk)
        parts.set(position, Buffer.concat(chunks))
        putPositions.push(position)
        // Reproduce the phone logs: the part is persisted, its response is lost.
        if (position !== 0) reply({ part: { position, size: parts.get(position).length } })
      } else if (request.url === '/v1/uploads/http-file' && request.method === 'GET') {
        polls++
        reply({ upload: { file_id: metadata.id, size: bytes.length,
          parts: [...parts].map(([position, data]) => ({ position, size: data.length })),
        } })
      } else if (request.url === '/v1/uploads/http-file/complete') {
        assert.deepEqual(Buffer.concat([...parts.values()]), Buffer.from(bytes))
        completions++
        reply({ file: { ...metadata, status: 'ready' } })
      } else {
        assert.fail(`Unexpected ${request.method} ${request.url}`)
      }
    } catch (error) {
      response.destroy(error)
      assert.fail(error)
    }
  })
  await new Promise((resolve, reject) => {
    listener.once('error', reject)
    listener.listen(0, '127.0.0.1', resolve)
  })
  t.after(async () => {
    listener.closeAllConnections()
    await new Promise((resolve) => listener.close(resolve))
  })
  const origin = `http://127.0.0.1:${listener.address().port}`
  const nativeFetch = globalThis.fetch
  t.mock.method(globalThis, 'fetch', (path, options) => nativeFetch(new URL(path, origin), options))
  const progress = []
  const ready = await uploadStoredFile({
    file: new File([bytes], 'iphone.zip', { type: 'application/zip' }), ownerId: 'owner', token: 'token',
    signal: AbortSignal.timeout(15_000), onPhase() {}, onPart() {}, onProgress: (value) => progress.push(value),
  })
  assert.equal(ready.status, 'ready')
  assert.deepEqual(putPositions, [0, 1])
  assert.deepEqual(progress, [0, partSize, bytes.length])
  assert.ok(polls >= 1)
  assert.equal(completions, 1)
})
