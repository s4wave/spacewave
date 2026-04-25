// Worker that receives WebRTC objects and reports back.

self.onmessage = async function (e) {
  const { test, payload, transfer } = e.data

  if (test === 'dc-transfer') {
    // Received a directly transferred RTCDataChannel
    const dc = payload.dataChannel
    try {
      const info = {
        test,
        result: 'received',
        type: typeof dc,
        constructorName: dc?.constructor?.name || 'unknown',
        readyState: dc?.readyState || 'unknown',
        label: dc?.label || 'unknown',
      }
      self.postMessage(info)

      function setupDC(dc) {
        dc.onmessage = (ev) => {
          self.postMessage({
            test,
            result: 'data-received',
            data:
              typeof ev.data === 'string'
                ? ev.data
                : 'binary:' + ev.data.byteLength,
          })
        }
        dc.onclose = () => {
          self.postMessage({ test, result: 'closed' })
        }
        dc.send('hello from worker via transferred dc')
        self.postMessage({ test, result: 'sent' })
      }

      if (dc.readyState === 'open') {
        setupDC(dc)
      } else {
        // DC arrived in connecting state (early transfer). Wait for open.
        dc.onopen = () => {
          self.postMessage({ test, result: 'opened', readyState: dc.readyState })
          setupDC(dc)
        }
        dc.onerror = (ev) => {
          self.postMessage({ test, result: 'error', error: 'dc error: ' + (ev.error?.message || ev.message || 'unknown') })
        }
      }
    } catch (err) {
      self.postMessage({ test, result: 'error', error: err.message })
    }
  }

  if (test === 'dc-transfer-options') {
    // Same but received via {transfer: [...]} syntax
    const dc = payload.dataChannel
    try {
      self.postMessage({
        test,
        result: 'received',
        type: typeof dc,
        constructorName: dc?.constructor?.name || 'unknown',
        readyState: dc?.readyState || 'unknown',
        label: dc?.label || 'unknown',
      })

      function setupOptionsDC(dc) {
        dc.onmessage = (ev) => {
          self.postMessage({
            test,
            result: 'data-received',
            data:
              typeof ev.data === 'string'
                ? ev.data
                : 'binary:' + ev.data.byteLength,
          })
        }
        dc.send('hello from worker via options-transferred dc')
        self.postMessage({ test, result: 'sent' })
      }

      if (dc.readyState === 'open') {
        setupOptionsDC(dc)
      } else {
        dc.onopen = () => {
          self.postMessage({ test, result: 'opened', readyState: dc.readyState })
          setupOptionsDC(dc)
        }
      }
    } catch (err) {
      self.postMessage({ test, result: 'error', error: err.message })
    }
  }

  if (test === 'dc-top-level') {
    // DataChannel sent as the top-level message (not nested in payload)
    const dc = e.data.dataChannel
    try {
      self.postMessage({
        test,
        result: 'received',
        type: typeof dc,
        constructorName: dc?.constructor?.name || 'unknown',
        readyState: dc?.readyState || 'unknown',
        label: dc?.label || 'unknown',
      })

      function setupTopLevelDC(dc) {
        dc.onmessage = (ev) => {
          self.postMessage({
            test,
            result: 'data-received',
            data:
              typeof ev.data === 'string'
                ? ev.data
                : 'binary:' + ev.data.byteLength,
          })
        }
        dc.send('hello from worker via top-level dc')
        self.postMessage({ test, result: 'sent' })
      }

      if (dc.readyState === 'open') {
        setupTopLevelDC(dc)
      } else {
        dc.onopen = () => {
          self.postMessage({ test, result: 'opened', readyState: dc.readyState })
          setupTopLevelDC(dc)
        }
      }
    } catch (err) {
      self.postMessage({ test, result: 'error', error: err.message })
    }
  }

  if (test === 'msgport-pipe') {
    // MessagePort piped to a DataChannel on main thread
    const port = payload.port
    try {
      self.postMessage({
        test,
        result: 'received',
        type: typeof port,
        constructorName: port?.constructor?.name || 'unknown',
      })

      port.onmessage = (ev) => {
        const data = ev.data
        if (data instanceof ArrayBuffer) {
          self.postMessage({
            test,
            result: 'data-received',
            data: 'binary:' + data.byteLength,
            bytes: Array.from(new Uint8Array(data)),
          })
          // Echo binary back
          port.postMessage(data, [data])
        } else {
          self.postMessage({ test, result: 'data-received', data })
          port.postMessage('echo:' + data)
        }
      }
      port.start()

      port.postMessage('hello from worker via msgport')
      self.postMessage({ test, result: 'sent' })
    } catch (err) {
      self.postMessage({ test, result: 'error', error: err.message })
    }
  }

  if (test === 'pc-transfer') {
    const pc = payload.peerConnection
    try {
      self.postMessage({
        test,
        result: 'received',
        type: typeof pc,
        constructorName: pc?.constructor?.name || 'unknown',
        signalingState: pc?.signalingState || 'unknown',
      })
    } catch (err) {
      self.postMessage({ test, result: 'error', error: err.message })
    }
  }
}
