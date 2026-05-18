import type { BackendAPI } from '@aptre/bldr-sdk'
import type { TestbedRoot } from '../../../sdk/testbed/testbed.js'
import { EngineWorldState } from '../../../sdk/world/engine-state.js'
import { InitUnixFSOp } from '../../space/world/ops/ops.pb.js'
import { INIT_UNIXFS_OP_ID } from '../../space/world/ops/init-unixfs.js'
import { FSHandle } from '../../../sdk/unixfs/handle.js'
import { MknodType } from '../../../sdk/unixfs/handle.pb.js'

export default async function main(
  _backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  testbedRoot: TestbedRoot,
) {
  using engine = await testbedRoot.createWorld('test-unixfs-typed-object')
  const worldState = new EngineWorldState(engine, true)
  const tx = await worldState.getEngine().newTransaction(true, abortSignal)
  let committed = false

  try {
    await tx.applyWorldOp(
      INIT_UNIXFS_OP_ID,
      InitUnixFSOp.toBinary({
        objectKey: 'fs/quickjs-test',
        timestamp: new Date(),
      }),
      '',
      abortSignal,
    )

    const access = await tx.accessTypedObject('fs/quickjs-test', abortSignal)
    if (!access.resourceId) {
      throw new Error('accessTypedObject returned empty resource id')
    }

    using root = new FSHandle(tx.getResourceRef().createRef(access.resourceId))
    await root.getNodeType(abortSignal)
    await root.mknod(['probe.txt'], MknodType.FILE, 0o644, false, abortSignal)
    using file = await root.lookup('probe.txt', abortSignal)
    const written = await file.writeAt(
      0n,
      new TextEncoder().encode('quickjs unixfs typed object'),
      abortSignal,
    )
    if (written === 0n) {
      throw new Error('writeAt reported zero bytes')
    }

    await tx.commit(abortSignal)
    committed = true
  } finally {
    if (!committed) {
      await tx.discard(abortSignal).catch(() => {})
    }
    tx.release()
  }
}
