import { bytesToHex } from '@noble/hashes/utils.js'
import { describe, expect, it } from 'vitest'

import {
  hashSOCheckpointInner,
  verifySOCheckpoint,
  verifySOCheckpointAuthority,
} from './checkpoint.js'
import {
  SOConfigChangeError,
  verifySOConfigChain,
  verifySOConfigChange,
} from './config-chain.js'
import { hashSOControlMessageInner, verifySOControlMessage } from './control.js'
import {
  hashSOOperationInner,
  SOOperationSet,
  verifySOOperation,
} from './operation-log.js'
import { hashSOSequenceInner, verifySOSequence } from './sequence.js'
import {
  SharedObjectConfig,
  SOCheckpoint,
  SOConfigChange,
  SOControlMessage,
  SOOperation,
  SOSequence,
} from './sobject.pb.js'
import vectors from './testdata/operation-log-vectors.json'

// fromBase64 decodes the standard base64 Go writes for byte fields.
function fromBase64(data: string): Uint8Array {
  return Uint8Array.from(atob(data), (c) => c.charCodeAt(0))
}

// rejectionKind runs verify and returns the rejection kind, or '' on success.
async function rejectionKind(verify: () => Promise<unknown>): Promise<string> {
  try {
    await verify()
    return ''
  } catch (err) {
    expect(err).toBeInstanceOf(SOConfigChangeError)
    return (err as SOConfigChangeError).kind
  }
}

const objectID = vectors.sharedObjectId
const operations = new Map(
  vectors.operations.map((v) => [
    v.name,
    SOOperation.fromBinary(fromBase64(v.operation)),
  ]),
)

describe('operation vectors', () => {
  it.each(vectors.operations)('$name verifies as Go does', async (v) => {
    // Verify the operation under the vector object.
    const op = operations.get(v.name)!
    const verified = await verifySOOperation(objectID, op).then(
      () => true,
      () => false,
    )

    // A valid operation has Go's hash; an invalid one is rejected.
    expect(verified).toBe(v.hash !== '')
    if (verified) {
      expect(bytesToHex(hashSOOperationInner(op.inner!))).toBe(v.hash)
    }
  })

  it.each(vectors.operationSets)(
    '$name yields the same set in any order',
    async (v) => {
      for (const names of [v.operations, [...v.operations].reverse()]) {
        // Build the set in this order.
        const set = new SOOperationSet(objectID)
        for (const name of names) {
          await set.add(operations.get(name)!)
        }

        // The heads and evidence match Go.
        expect(set.heads()).toEqual(v.heads)
        expect(
          set.equivocations().map((e) => ({ ...e, nonce: Number(e.nonce) })),
        ).toEqual(v.equivocations)
      }
    },
  )
})

describe('control record vectors', () => {
  it.each(vectors.configChains)('chain $name', async (v) => {
    // Verify the chain from genesis.
    const entries = v.entries.map((e) =>
      SOConfigChange.fromBinary(fromBase64(e)),
    )
    let head = ''
    const kind = await rejectionKind(async () => {
      const cfg = await verifySOConfigChain(objectID, entries)
      head = bytesToHex(cfg.configChainHash!)
    })

    // The result matches Go and the expected rejection kind.
    expect(kind).toBe(v.kind)
    expect(head).toBe(v.headHash)
  })

  it.each(vectors.configChanges)('change $name', async (v) => {
    // Apply the record to the held configuration.
    const current = SharedObjectConfig.fromBinary(fromBase64(v.current))
    const entry = SOConfigChange.fromBinary(fromBase64(v.entry))
    let next = ''
    const kind = await rejectionKind(async () => {
      const cfg = await verifySOConfigChange(objectID, current, entry)
      next = bytesToHex(cfg.configChainHash!)
    })

    // The result matches Go and the expected rejection kind.
    expect(kind).toBe(v.kind)
    expect(next).toBe(v.nextHash)
  })
})

describe('checkpoint vectors', () => {
  it.each(vectors.checkpoints)('$name verifies as Go does', async (v) => {
    // Verify the checkpoint under the vector object.
    const checkpoint = SOCheckpoint.fromBinary(fromBase64(v.checkpoint))
    const verified = await verifySOCheckpoint(objectID, checkpoint).then(
      () => true,
      () => false,
    )

    // A valid checkpoint has Go's hash; an invalid one is rejected.
    expect(verified).toBe(v.hash !== '')
    if (verified) {
      expect(bytesToHex(hashSOCheckpointInner(checkpoint.inner!))).toBe(v.hash)
    }

    // Authority matches Go under the vector config.
    const config = SharedObjectConfig.fromBinary(fromBase64(v.config))
    const authority = await verifySOCheckpointAuthority(
      objectID,
      checkpoint,
      config,
    ).then(
      () => true,
      () => false,
    )
    expect(authority).toBe(v.authority)
  })
})

describe('control message vectors', () => {
  it.each(vectors.controlMessages)('$name verifies as Go does', async (v) => {
    // Verify the message under the vector object.
    const msg = SOControlMessage.fromBinary(fromBase64(v.message))
    const inner = await verifySOControlMessage(objectID, msg).then(
      (inner) => inner,
      () => null,
    )

    // A valid message has Go's hash and voter; an invalid one is rejected.
    expect(inner?.peerId ?? '').toBe(v.peerId)
    if (inner) {
      expect(bytesToHex(hashSOControlMessageInner(msg.inner!))).toBe(v.hash)
    }
  })
})

describe('sequence vectors', () => {
  it.each(vectors.sequences)('$name verifies as Go does', async (v) => {
    // Verify the position under the vector object.
    const record = SOSequence.fromBinary(fromBase64(v.sequence))
    const inner = await verifySOSequence(objectID, record).then(
      (inner) => inner,
      () => null,
    )

    // A valid position has Go's hash and signer; an invalid one is rejected.
    expect(inner !== null).toBe(v.hash !== '')
    if (inner) {
      expect(bytesToHex(hashSOSequenceInner(record.inner!))).toBe(v.hash)
      expect(inner.peerId).toBe(v.signer)
    }
  })
})
