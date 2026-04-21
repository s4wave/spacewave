onconnect = function (e) {
  const port = e.ports[0]
  port.onmessage = function (event) {
    const workerID = event.data
    const port = event.ports[0]
    handlePort(workerID, port)
  }
}

function handlePort(workerID, port) {
  console.log('SharedWorker onconnect', workerID, port)
  port.onmessage = function (event) {
    console.log(workerID, 'onmessage', event.data, event.ports)
    if (event.ports.length) {
      event.ports[0].onmessage = function (event) {
        console.log(
          workerID,
          'got message from other worker',
          event.data,
          event.ports,
        )
      }
      event.ports[0].start()
      event.ports[0].postMessage('hello from ' + workerID + ' to other worker!')
    }
  }
  port.start()
}


// calling self.close() removes the SharedWorker
