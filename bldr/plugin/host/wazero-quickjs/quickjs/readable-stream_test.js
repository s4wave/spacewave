// readable-stream_test.js - runs the ReadableStream polyfill inside the real
// QuickJS WASM engine via polyfill_test.go.

import { ReadableStream } from './readable-stream.js'

console.log('Starting ReadableStream polyfill tests...')

function assert(cond, msg) {
  if (!cond) {
    throw new Error('assertion failed: ' + msg)
  }
}

function bytesEqual(a, b) {
  if (a.byteLength !== b.byteLength) {
    return false
  }
  for (let i = 0; i < a.byteLength; i++) {
    if (a[i] !== b[i]) {
      return false
    }
  }
  return true
}

async function drain(stream) {
  const reader = stream.getReader()
  const chunks = []
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) {
        break
      }
      chunks.push(value)
    }
  } finally {
    reader.releaseLock()
  }
  return chunks
}

assert(
  typeof ReadableStream === 'function',
  'ReadableStream constructor exists',
)

// Test 1: synchronous start enqueue + close round-trips (uploadFile pattern).
{
  const data = new Uint8Array([1, 2, 3, 4])
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(data)
      controller.close()
    },
  })
  const chunks = await drain(stream)
  assert(chunks.length === 1, 'one chunk read, got ' + chunks.length)
  assert(bytesEqual(chunks[0], data), 'chunk bytes match')
  console.log('start/enqueue/close round-trip works')
}

// Test 2: multiple chunks preserve order.
{
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new Uint8Array([10]))
      controller.enqueue(new Uint8Array([20]))
      controller.enqueue(new Uint8Array([30]))
      controller.close()
    },
  })
  const chunks = await drain(stream)
  assert(chunks.length === 3, 'three chunks read, got ' + chunks.length)
  assert(
    chunks[0][0] === 10 && chunks[1][0] === 20 && chunks[2][0] === 30,
    'chunk order preserved',
  )
  console.log('multi-chunk ordering works')
}

// Test 3: pull-based source feeds chunks on demand.
{
  let pulls = 0
  const stream = new ReadableStream({
    pull(controller) {
      pulls++
      if (pulls <= 2) {
        controller.enqueue(new Uint8Array([pulls]))
      } else {
        controller.close()
      }
    },
  })
  const chunks = await drain(stream)
  assert(chunks.length === 2, 'two pulled chunks, got ' + chunks.length)
  assert(
    chunks[0][0] === 1 && chunks[1][0] === 2,
    'pulled chunk values correct',
  )
  console.log('pull-based source works')
}

// Test 4: error propagates to the reader.
{
  const stream = new ReadableStream({
    start(controller) {
      controller.error(new Error('boom'))
    },
  })
  const reader = stream.getReader()
  let threw = false
  try {
    await reader.read()
  } catch (err) {
    threw = true
    assert(String(err.message) === 'boom', 'error message propagated')
  }
  assert(threw, 'read rejected on errored stream')
  console.log('error propagation works')
}

// Test 5: locking is exclusive.
{
  const stream = new ReadableStream({
    start(controller) {
      controller.close()
    },
  })
  stream.getReader()
  assert(stream.locked === true, 'stream is locked after getReader')
  let threw = false
  try {
    stream.getReader()
  } catch (_err) {
    threw = true
  }
  assert(threw, 'second getReader throws while locked')
  console.log('exclusive locking works')
}

console.log('\nAll ReadableStream polyfill tests passed!')
