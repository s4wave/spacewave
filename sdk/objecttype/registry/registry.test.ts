import { describe, it, expect } from 'vitest'
import {
  ObjectTypeRegistration,
  RegisterObjectTypeRequest,
  RegisterObjectTypeResponse,
  WatchObjectTypesResponse,
  InvokeObjectTypeRequest,
  InvokeObjectTypeResponse,
} from './registry.pb.js'
import { ObjectTypeRegistry } from './registry.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

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

describe('ObjectTypeRegistry proto types', () => {
  it('ObjectTypeRegistration has typeId, registrationId, pluginId fields', () => {
    const reg = ObjectTypeRegistration.create({
      typeId: 'test-type',
      registrationId: 7,
      pluginId: 'my-plugin',
    })
    expect(reg.typeId).toBe('test-type')
    expect(reg.registrationId).toBe(7)
    expect(reg.pluginId).toBe('my-plugin')
  })

  it('RegisterObjectTypeRequest has typeId and pluginId fields', () => {
    const req = RegisterObjectTypeRequest.create({
      typeId: 'notes/notebook',
      pluginId: 'notes-plugin',
    })
    expect(req.typeId).toBe('notes/notebook')
    expect(req.pluginId).toBe('notes-plugin')
  })

  it('RegisterObjectTypeResponse has resourceId field', () => {
    const resp = RegisterObjectTypeResponse.create({ resourceId: 99 })
    expect(resp.resourceId).toBe(99)
  })

  it('WatchObjectTypesResponse has registrations array', () => {
    const resp = WatchObjectTypesResponse.create({
      registrations: [
        { typeId: 'type-a', registrationId: 1, pluginId: 'plugin-a' },
        { typeId: 'type-b', registrationId: 2, pluginId: 'plugin-b' },
      ],
    })
    expect(resp.registrations).toHaveLength(2)
    expect(resp.registrations![0].typeId).toBe('type-a')
    expect(resp.registrations![1].pluginId).toBe('plugin-b')
  })

  it('ObjectTypeRegistration round-trip serialization', () => {
    const original = ObjectTypeRegistration.create({
      typeId: 'spacewave-notes/notebook',
      registrationId: 42,
      pluginId: 'notes-plugin',
    })
    const bytes = ObjectTypeRegistration.toBinary(original)
    const decoded = ObjectTypeRegistration.fromBinary(bytes)
    expect(decoded.typeId).toBe('spacewave-notes/notebook')
    expect(decoded.registrationId).toBe(42)
    expect(decoded.pluginId).toBe('notes-plugin')
  })
})

describe('ObjectTypeRegistry SDK class', () => {
  it('constructs with mock ClientResourceRef', () => {
    const ref = createMockRef()
    const registry = new ObjectTypeRegistry(ref)
    expect(registry).toBeDefined()
    expect(registry.id).toBe(42)
  })

  it('extends Resource base class', () => {
    const ref = createMockRef()
    const registry = new ObjectTypeRegistry(ref)
    expect(registry).toBeInstanceOf(Resource)
  })

  it('has registerObjectType method', () => {
    const ref = createMockRef()
    const registry = new ObjectTypeRegistry(ref)
    expect(typeof registry.registerObjectType).toBe('function')
  })

  it('has watchObjectTypes method', () => {
    const ref = createMockRef()
    const registry = new ObjectTypeRegistry(ref)
    expect(typeof registry.watchObjectTypes).toBe('function')
  })
})

describe('ObjectTypeHandlerService proto types', () => {
  it('InvokeObjectTypeRequest has typeId, objectKey, engineResourceId', () => {
    const req = InvokeObjectTypeRequest.create({
      typeId: 'notes/notebook',
      objectKey: 'obj-123',
      engineResourceId: 5,
    })
    expect(req.typeId).toBe('notes/notebook')
    expect(req.objectKey).toBe('obj-123')
    expect(req.engineResourceId).toBe(5)
  })

  it('InvokeObjectTypeResponse has resourceId', () => {
    const resp = InvokeObjectTypeResponse.create({ resourceId: 10 })
    expect(resp.resourceId).toBe(10)
  })

  it('InvokeObjectTypeRequest round-trip serialization', () => {
    const original = InvokeObjectTypeRequest.create({
      typeId: 'canvas/board',
      objectKey: 'world-obj-456',
      engineResourceId: 88,
    })
    const bytes = InvokeObjectTypeRequest.toBinary(original)
    const decoded = InvokeObjectTypeRequest.fromBinary(bytes)
    expect(decoded.typeId).toBe('canvas/board')
    expect(decoded.objectKey).toBe('world-obj-456')
    expect(decoded.engineResourceId).toBe(88)
  })
})
