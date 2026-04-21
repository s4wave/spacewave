// SharedWorker that relays SAB between tabs.
// Tab A stores SAB, Tab B requests it.
let storedSab = null
const ports = []

onconnect = (e) => {
  const port = e.ports[0]
  ports.push(port)
  port.onmessage = (ev) => {
    if (ev.data?.type === 'store') {
      storedSab = ev.data.sab
    }
    if (ev.data?.type === 'request') {
      if (storedSab) {
        port.postMessage({ type: 'sab-relay', sab: storedSab })
      } else {
        port.postMessage({ type: 'no-sab' })
      }
    }
  }
  port.start()
}
