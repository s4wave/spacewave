import net from 'net'
import { buildPipeName } from './pipesock.js'

// pipesock_test.ts is a test client for validating Go<->TypeScript socket communication.
// Usage: bun pipesock_test.ts <rootDir> <pipeUuid>
//
// The script:
// 1. Connects to the Unix socket at buildPipeName(rootDir, pipeUuid)
// 2. Sends a test message
// 3. Reads the echoed response
// 4. Exits with code 0 on success, 1 on failure

const args = process.argv.slice(2)
if (args.length < 2) {
  console.error('Usage: bun pipesock_test.ts <rootDir> <pipeUuid>')
  process.exit(1)
}

const [rootDir, pipeUuid] = args
const pipePath = buildPipeName(rootDir, pipeUuid)

console.log(`[ts-client] connecting to pipe: ${pipePath}`)
console.log(`[ts-client] rootDir: ${rootDir}`)
console.log(`[ts-client] pipeUuid: ${pipeUuid}`)
console.log(`[ts-client] cwd: ${process.cwd()}`)

const testMessage = 'hello from typescript'
const expectedResponse = `echo: ${testMessage}`

const socket = net.connect(pipePath, () => {
  console.log(`[ts-client] connected to pipe`)

  // Send test message
  socket.write(testMessage)
  console.log(`[ts-client] sent: ${testMessage}`)
})

let receivedData = ''

socket.on('data', (data) => {
  receivedData += data.toString()
  console.log(`[ts-client] received: ${receivedData}`)

  // Check if we got the expected response
  if (receivedData === expectedResponse) {
    console.log('[ts-client] success: received expected response')
    socket.end()
    process.exit(0)
  } else if (receivedData.length >= expectedResponse.length) {
    console.error(
      `[ts-client] error: expected "${expectedResponse}", got "${receivedData}"`,
    )
    socket.end()
    process.exit(1)
  }
})

socket.on('error', (err) => {
  console.error(`[ts-client] error: ${err.message}`)
  process.exit(1)
})

socket.on('close', () => {
  console.log('[ts-client] connection closed')
  if (receivedData !== expectedResponse) {
    console.error(
      `[ts-client] error: connection closed before receiving expected response`,
    )
    process.exit(1)
  }
})

// Timeout after 10 seconds
setTimeout(() => {
  console.error('[ts-client] error: timeout waiting for response')
  socket.destroy()
  process.exit(1)
}, 10000)
