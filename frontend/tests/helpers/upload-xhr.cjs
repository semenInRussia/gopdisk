// Bridge XHR uploads to the existing HTTP test servers. Individual tests may
// emit progress events or deliberately withhold the response.
module.exports = function installUploadXHR(t) {
  const previous = globalThis.XMLHttpRequest
  const requests = []
  globalThis.XMLHttpRequest = class {
    upload = {}
    headers = new Headers()
    controller = new AbortController()
    open(method, path) { this.method = method; this.path = path }
    setRequestHeader(name, value) { this.headers.set(name, value) }
    abort() { this.controller.abort(); this.onabort?.() }
    send(body) {
      this.body = body
      requests.push(this)
      void this.transfer(body)
    }
    async transfer(body) {
      try {
        const response = await fetch(this.path, {
          method: this.method, headers: this.headers, body, signal: this.controller.signal, cache: 'no-store',
        })
        this.status = response.status
        this.responseText = await response.text()
        this.onload?.()
      } catch {
        if (this.controller.signal.aborted) this.onabort?.()
        else this.onerror?.()
      }
    }
  }
  t.after(() => {
    if (previous === undefined) delete globalThis.XMLHttpRequest
    else globalThis.XMLHttpRequest = previous
  })
  return requests
}
