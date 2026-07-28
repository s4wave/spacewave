import { describe, expect, it } from 'vitest'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { CommandsManager } from './registry.js'

function createMockRef(): ClientResourceRef {
  return {
    resourceId: 42,
    released: false,
    client: {},
    createRef: () => createMockRef(),
    createResource: () => null,
    release: () => {},
    [Symbol.dispose]: () => {},
  } as unknown as ClientResourceRef
}

describe('CommandsManager SDK class', () => {
  it('constructs without a surface for legacy web callers', () => {
    const manager = new CommandsManager(createMockRef())
    expect(manager).toBeInstanceOf(Resource)
    expect(manager.id).toBe(42)
  })
})
