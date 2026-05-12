// sab-ring.ts - SAB ring buffer point-to-point test fixture.
//
// Creates a SabRingStream pair, sends messages, verifies ordering,
// tests bidirectional and close propagation.

import {
  SAB_PAIR_DIRECTION_MTU_BYTES,
  SabRingStream,
  createSabPair,
  sabBufferSize,
  sabPairBufferSize,
} from '../../../web/bldr/sab-ring-stream.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      sendRecv: boolean
      bidirectional: boolean
      close: boolean
      pairDefault: boolean
      maxPayload: boolean
      messageCount: number
    }
  }
}

async function collectN(
  source: AsyncIterable<Uint8Array>,
  n: number,
  timeoutMs: number,
): Promise<Uint8Array[]> {
  const msgs: Uint8Array[] = []
  const deadline = Date.now() + timeoutMs
  for await (const chunk of source) {
    msgs.push(new Uint8Array(chunk))
    if (msgs.length >= n) break
    if (Date.now() > deadline) break
  }
  return msgs
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    const expectedDirectionBytes = 16 + SAB_PAIR_DIRECTION_MTU_BYTES + 4
    const pairDefault =
      sabBufferSize() === expectedDirectionBytes &&
      sabPairBufferSize() === 2 * expectedDirectionBytes

    // Test 1: Send 10 messages A->B, verify all received in order.
    let sendRecv = false
    {
      const { aSab, bSab } = createSabPair()
      const streamA = new SabRingStream(aSab, bSab)
      const streamB = new SabRingStream(bSab, aSab)

      const count = 10
      const recvPromise = collectN(streamB.source, count, 5000)

      const writeDone = streamA.sink(
        (async function* () {
          for (let i = 0; i < count; i++) {
            yield new Uint8Array([i])
          }
        })(),
      )

      const received = await recvPromise
      await writeDone
      if (received.length !== count) {
        errors.push(`sendRecv: got ${received.length} msgs, want ${count}`)
      } else {
        let ok = true
        for (let i = 0; i < count; i++) {
          if (received[i][0] !== i) {
            errors.push(`sendRecv: msg[${i}]=${received[i][0]}, want ${i}`)
            ok = false
            break
          }
        }
        sendRecv = ok
      }

      streamA.close()
      streamB.close()
    }

    // Test 2: Bidirectional - both sides send simultaneously.
    let bidirectional = false
    {
      const { aSab, bSab } = createSabPair()
      const streamA = new SabRingStream(aSab, bSab)
      const streamB = new SabRingStream(bSab, aSab)

      const count = 5
      const recvA = collectN(streamA.source, count, 5000)
      const recvB = collectN(streamB.source, count, 5000)

      // A sends 0xAA bytes, B sends 0xBB bytes.
      const doneA = streamA.sink(
        (async function* () {
          for (let i = 0; i < count; i++) {
            yield new Uint8Array([0xaa, i])
          }
        })(),
      )
      const doneB = streamB.sink(
        (async function* () {
          for (let i = 0; i < count; i++) {
            yield new Uint8Array([0xbb, i])
          }
        })(),
      )

      const msgsA = await recvA
      const msgsB = await recvB
      await Promise.all([doneA, doneB])

      if (msgsA.length === count && msgsB.length === count) {
        let ok = true
        for (let i = 0; i < count; i++) {
          if (msgsB[i][0] !== 0xaa || msgsB[i][1] !== i) {
            errors.push(`bidir: B got wrong msg at ${i}`)
            ok = false
            break
          }
          if (msgsA[i][0] !== 0xbb || msgsA[i][1] !== i) {
            errors.push(`bidir: A got wrong msg at ${i}`)
            ok = false
            break
          }
        }
        bidirectional = ok
      } else {
        errors.push(
          `bidir: A got ${msgsA.length}, B got ${msgsB.length}, want ${count}`,
        )
      }

      streamA.close()
      streamB.close()
    }

    // Test 3: Close propagation - closing A's sink should end B's source.
    let closeOk = false
    {
      const { aSab, bSab } = createSabPair()
      const streamA = new SabRingStream(aSab, bSab)
      const streamB = new SabRingStream(bSab, aSab)

      // Send one message then close.
      await streamA.sink(
        (async function* () {
          yield new Uint8Array([42])
        })(),
      )
      streamA.close()

      // B should receive the message and then the source should end.
      const msgs: Uint8Array[] = []
      const deadline = Date.now() + 3000
      for await (const chunk of streamB.source) {
        msgs.push(new Uint8Array(chunk))
        if (Date.now() > deadline) break
      }

      if (msgs.length >= 1 && msgs[0][0] === 42) {
        closeOk = true
      } else {
        errors.push(`close: got ${msgs.length} msgs, first=${msgs[0]?.[0]}`)
      }

      streamB.close()
    }

    // Test 4: Default pair accepts exactly 32 KiB payloads.
    let maxPayload = false
    {
      const { aSab, bSab } = createSabPair()
      const streamA = new SabRingStream(aSab, bSab)
      const streamB = new SabRingStream(bSab, aSab)
      const msg = new Uint8Array(SAB_PAIR_DIRECTION_MTU_BYTES)
      msg[0] = 0xab
      msg[msg.byteLength - 1] = 0xcd

      const writeDone = streamA.sink(
        (async function* () {
          yield msg
        })(),
      )
      const received = await streamB.source.next()
      await writeDone

      maxPayload =
        received.done === false &&
        received.value.byteLength === SAB_PAIR_DIRECTION_MTU_BYTES &&
        received.value[0] === 0xab &&
        received.value[received.value.byteLength - 1] === 0xcd
      if (!maxPayload) {
        errors.push('maxPayload: did not receive exact default MTU payload')
      }

      streamA.close()
      streamB.close()
    }

    const pass =
      pairDefault &&
      sendRecv &&
      bidirectional &&
      closeOk &&
      maxPayload &&
      errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      sendRecv,
      bidirectional,
      close: closeOk,
      pairDefault,
      maxPayload,
      messageCount: 10,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      sendRecv: false,
      bidirectional: false,
      close: false,
      pairDefault: false,
      maxPayload: false,
      messageCount: 0,
    }
  }

  log.textContent = 'DONE'
}

run()
