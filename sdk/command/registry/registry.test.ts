import { CommandSurface } from '../command.pb.js'
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
  it('constructs only with an explicit surface', () => {
    expect(() => new CommandsManager(createMockRef())).toThrow(
      'command surface must be WEB or TUI',
    )
    expect(
      () => new CommandsManager(createMockRef(), 99 as CommandSurface),
    ).toThrow('command surface must be WEB or TUI')
    const manager = new CommandsManager(createMockRef(), CommandSurface.WEB)
    expect(manager).toBeInstanceOf(Resource)
    expect(manager.id).toBe(42)
  })
})
