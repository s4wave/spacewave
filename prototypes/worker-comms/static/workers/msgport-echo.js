// Simple echo worker: receives a message, posts it back.
postMessage('ready')
onmessage = (e) => {
  postMessage(e.data)
}
