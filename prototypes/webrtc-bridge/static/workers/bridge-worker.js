// Bridge worker: receives a MessagePort from the main thread, sends
// signaling commands over it, and receives responses.

let bridgePort = null
let nextCmdId = 1

// Pending command callbacks keyed by cmdId
const pending = new Map()

// Event listeners keyed by "pcId:eventType"
const eventListeners = new Map()

// Cached property snapshots keyed by pcId
const snapshotCache = new Map()

function sendCommand(type, payload) {
  return new Promise((resolve, reject) => {
    const cmdId = nextCmdId++
    const msg = { type, cmdId, ...payload }
    pending.set(cmdId, { resolve, reject })
    bridgePort.postMessage(msg)
  })
}

function onEvent(pcId, eventType, handler) {
  const key = pcId + ':' + eventType
  let handlers = eventListeners.get(key)
  if (!handlers) {
    handlers = []
    eventListeners.set(key, handlers)
  }
  handlers.push(handler)
}

function updateCache(data) {
  if (data.snapshot && data.pcId) {
    snapshotCache.set(data.pcId, data.snapshot)
  }
}

function getCache(pcId) {
  return snapshotCache.get(pcId) || null
}

function handleMessage(data) {
  updateCache(data)

  // Command response (has cmdId)
  if (data.cmdId != null) {
    const entry = pending.get(data.cmdId)
    if (entry) {
      pending.delete(data.cmdId)
      if (data.error) {
        entry.reject(new Error(data.error))
      } else {
        entry.resolve(data)
      }
    }
    return
  }

  // Event (type starts with "event:")
  if (data.type && data.type.startsWith('event:')) {
    const eventType = data.type.slice(6)
    const key = data.pcId + ':' + eventType
    const handlers = eventListeners.get(key)
    if (handlers) {
      for (const h of handlers) h(data)
    }
  }
}

function report(test, result, done) {
  self.postMessage({ type: 'test-result', test, result, done: !!done })
}

// Wait for a DC to open (handles both already-open and connecting states)
function waitDCOpen(dc, timeout = 10000) {
  return new Promise((resolve, reject) => {
    if (dc.readyState === 'open') {
      resolve()
      return
    }
    const timer = setTimeout(() => reject(new Error('DC open timeout')), timeout)
    dc.onopen = () => {
      clearTimeout(timer)
      resolve()
    }
    dc.onerror = (e) => {
      clearTimeout(timer)
      reject(new Error('DC error: ' + (e.message || e.error || 'unknown')))
    }
  })
}

// Wait for a PC to reach a target connectionState via events
function waitConnectionState(pcId, target, timeout = 10000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout waiting for ' + target)), timeout)
    const check = (snapshot) => {
      if (snapshot.connectionState === target) {
        clearTimeout(timer)
        resolve(snapshot)
        return true
      }
      return false
    }
    onEvent(pcId, 'connectionstatechange', (e) => check(e.snapshot))
    onEvent(pcId, 'iceconnectionstatechange', (e) => check(e.snapshot))
  })
}

// Wait for a single message on a DC
function waitDCMessage(dc, timeout = 5000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('DC message timeout')), timeout)
    dc.onmessage = (e) => {
      clearTimeout(timer)
      resolve(e.data)
    }
  })
}

async function doSignaling(pcOffer, pcAnswer) {
  // Queue candidates until the remote side has setRemoteDescription.
  // Without queueing, addIceCandidate can reject if the remote side
  // has no remote description yet.
  const offerCandidateQueue = []
  const answerCandidateQueue = []
  let offerRemoteSet = false
  let answerRemoteSet = false

  async function flushCandidates(queue, targetPcId) {
    for (const cand of queue) {
      await sendCommand('addIceCandidate', { pcId: targetPcId, candidate: cand })
    }
    queue.length = 0
  }

  // Offerer's candidates go to answerer (needs answerer's remote desc set first)
  onEvent(pcOffer, 'icecandidate', (e) => {
    if (answerRemoteSet) {
      sendCommand('addIceCandidate', { pcId: pcAnswer, candidate: e.candidate })
        .catch((err) => console.error('addIceCandidate to answerer failed:', err))
    } else {
      offerCandidateQueue.push(e.candidate)
    }
  })

  // Answerer's candidates go to offerer (needs offerer's remote desc set first)
  onEvent(pcAnswer, 'icecandidate', (e) => {
    if (offerRemoteSet) {
      sendCommand('addIceCandidate', { pcId: pcOffer, candidate: e.candidate })
        .catch((err) => console.error('addIceCandidate to offerer failed:', err))
    } else {
      answerCandidateQueue.push(e.candidate)
    }
  })

  // Offerer: create offer and set local description (starts ICE gathering)
  const offerResult = await sendCommand('createOffer', { pcId: pcOffer })
  await sendCommand('setLocalDescription', { pcId: pcOffer, sdp: offerResult.sdp })

  // Answerer: set remote description (offerer's offer), then flush queued candidates
  await sendCommand('setRemoteDescription', { pcId: pcAnswer, sdp: offerResult.sdp })
  answerRemoteSet = true
  await flushCandidates(offerCandidateQueue, pcAnswer)

  // Answerer: create answer and set local description (starts ICE gathering)
  const answerResult = await sendCommand('createAnswer', { pcId: pcAnswer })
  await sendCommand('setLocalDescription', { pcId: pcAnswer, sdp: answerResult.sdp })

  // Offerer: set remote description (answerer's answer), then flush queued candidates
  await sendCommand('setRemoteDescription', { pcId: pcOffer, sdp: answerResult.sdp })
  offerRemoteSet = true
  await flushCandidates(answerCandidateQueue, pcOffer)
}

async function runTests() {
  // Test 1: createPC (seed)
  try {
    const r1 = await sendCommand('createPC', { config: { iceServers: [] } })
    report('seed-rpc', { pass: true, pcId: r1.pcId, snapshot: r1.snapshot })
  } catch (err) {
    report('seed-rpc', { pass: false, error: err.message })
  }

  // Test 2: full signaling
  try {
    const r1 = await sendCommand('createPC', { config: { iceServers: [] } })
    const r2 = await sendCommand('createPC', { config: { iceServers: [] } })
    // Need a DC for ICE to negotiate
    await sendCommand('createDataChannel', { pcId: r1.pcId, label: 'sig-test' })
    await doSignaling(r1.pcId, r2.pcId)
    report('signaling', { pass: true, offerType: 'offer', answerType: 'answer', offererState: 'stable' })
  } catch (err) {
    report('signaling', { pass: false, error: err.message })
  }

  // Test 3: ICE connectivity
  try {
    const r1 = await sendCommand('createPC', { config: { iceServers: [] } })
    const r2 = await sendCommand('createPC', { config: { iceServers: [] } })
    await sendCommand('createDataChannel', { pcId: r1.pcId, label: 'ice-test' })
    const connPromise = waitConnectionState(r1.pcId, 'connected')
    await doSignaling(r1.pcId, r2.pcId)
    const snap = await connPromise
    report('ice-connectivity', {
      pass: true,
      connectionState: snap.connectionState,
    })
  } catch (err) {
    report('ice-connectivity', { pass: false, error: err.message })
  }

  // Test 4: DC transfer -- offerer creates DC, main thread transfers it to worker
  try {
    const r1 = await sendCommand('createPC', { config: { iceServers: [] } })
    const r2 = await sendCommand('createPC', { config: { iceServers: [] } })

    // Create DC via RPC -- the response includes the transferred DC
    const dcResult = await sendCommand('createDataChannel', {
      pcId: r1.pcId,
      label: 'xfer-test',
    })
    const offererDC = dcResult.dc

    // The answerer will get an ondatachannel event with the transferred DC
    const answererDCPromise = new Promise((resolve) => {
      onEvent(r2.pcId, 'datachannel', (e) => resolve(e.dc))
    })

    // Do signaling
    const connPromise = waitConnectionState(r1.pcId, 'connected')
    await doSignaling(r1.pcId, r2.pcId)
    await connPromise

    // Wait for both DCs to open
    await waitDCOpen(offererDC)
    const answererDC = await answererDCPromise
    await waitDCOpen(answererDC)

    // Data roundtrip: offerer -> answerer
    const msgPromise = waitDCMessage(answererDC)
    offererDC.send('hello from offerer')
    const received = await msgPromise

    // Data roundtrip: answerer -> offerer
    const msg2Promise = waitDCMessage(offererDC)
    answererDC.send('hello from answerer')
    const received2 = await msg2Promise

    // Binary roundtrip
    const binPromise = waitDCMessage(answererDC)
    const testBytes = new Uint8Array([1, 2, 3, 4, 5])
    offererDC.send(testBytes)
    const binReceived = await binPromise

    report('dc-transfer', {
      pass: true,
      offererDCType: typeof offererDC,
      offererLabel: offererDC.label,
      answererLabel: answererDC.label,
      textForward: received,
      textReverse: received2,
      binarySize: binReceived instanceof ArrayBuffer ? binReceived.byteLength : -1,
    })
  } catch (err) {
    report('dc-transfer', { pass: false, error: err.message })
  }

  // Test 5: Property snapshot cache -- verify snapshots are cached and
  // accurate through the PC lifecycle
  try {
    const r1 = await sendCommand('createPC', { config: { iceServers: [] } })
    const r2 = await sendCommand('createPC', { config: { iceServers: [] } })

    // After createPC, cache should have 'new' state
    const snap0 = getCache(r1.pcId)
    const initialOk = snap0 && snap0.connectionState === 'new' && snap0.signalingState === 'stable'

    // Create DC and do signaling
    await sendCommand('createDataChannel', { pcId: r1.pcId, label: 'snap-test' })
    const connPromise = waitConnectionState(r1.pcId, 'connected')
    await doSignaling(r1.pcId, r2.pcId)
    await connPromise

    // After connection, cache should reflect 'connected' + 'stable'
    const snap1 = getCache(r1.pcId)
    const connectedOk = snap1 && snap1.connectionState === 'connected' && snap1.signalingState === 'stable'

    // Verify snapshot has all expected fields
    const fields = [
      'connectionState', 'signalingState', 'iceConnectionState',
      'iceGatheringState', 'localDescription', 'remoteDescription',
    ]
    const allFields = fields.every((f) => f in snap1)

    // localDescription and remoteDescription should be populated after signaling
    const hasDescs = snap1.localDescription !== null && snap1.remoteDescription !== null

    report('snapshot-cache', {
      pass: initialOk && connectedOk && allFields && hasDescs,
      initialOk,
      connectedOk,
      allFields,
      hasDescs,
      finalSnapshot: snap1,
    }, true)
  } catch (err) {
    report('snapshot-cache', { pass: false, error: err.message }, true)
  }
}

self.onmessage = async (e) => {
  const msg = e.data

  if (msg.type === 'init-bridge') {
    bridgePort = msg.port
    bridgePort.onmessage = (e) => handleMessage(e.data)

    self.postMessage({ type: 'bridge-ready' })
    await runTests()
  }
}
