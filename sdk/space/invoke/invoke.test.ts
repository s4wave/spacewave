import { describe, expect, it } from 'vitest'

import {
  AccessMode,
  InvokeSpaceRequest,
  InvokeSpaceResponse,
  SpaceInvocationEnvelope,
} from './invoke.pb.js'

describe('Space invocation contract', () => {
  it('round-trips grant lineage, lease, and attached bindings', () => {
    const original = SpaceInvocationEnvelope.create({
      spaceId: 'space-1',
      grantId: 'grant-1',
      grantVersion: 7n,
      principalId: 'plugin:notes',
      issuerId: 'space:space-1',
      leaseExpiresUnixMs: 1_800_000_000_000n,
      bindings: [
        {
          bindingId: 'database',
          typeId: 'sql/database',
          objectKey: 'database/main',
          serviceId: 's4wave.sql.SqlDatabaseResourceService',
          accessMode: AccessMode.READ_WRITE,
          attachedResourceId: 42,
        },
      ],
    })

    const decoded = SpaceInvocationEnvelope.fromBinary(
      SpaceInvocationEnvelope.toBinary(original),
    )

    expect(decoded).toEqual(original)
  })

  it('keeps the general invocation operation and result typed', () => {
    const request = InvokeSpaceRequest.create({
      envelope: {
        spaceId: 'space-1',
        grantId: 'grant-1',
        grantVersion: 7n,
      },
      operationId: 'notes/create',
      opData: Uint8Array.from([1, 2, 3]),
    })
    const response = InvokeSpaceResponse.create({
      resourceId: 43,
      resultData: Uint8Array.from([4, 5, 6]),
    })

    expect(
      InvokeSpaceRequest.fromBinary(InvokeSpaceRequest.toBinary(request)),
    ).toEqual(request)
    expect(
      InvokeSpaceResponse.fromBinary(InvokeSpaceResponse.toBinary(response)),
    ).toEqual(response)
  })
})
