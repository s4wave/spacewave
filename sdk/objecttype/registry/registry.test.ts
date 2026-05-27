import { describe, it, expect } from 'vitest'
import {
  ObjectTypeMetadata,
  ObjectTypeRegistration,
  ObjectTypeVisibility,
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
  it('ObjectTypeMetadata has display fields and visibility', () => {
    const metadata = ObjectTypeMetadata.create({
      displayName: 'Notebook',
      iconName: 'notebook',
      visibility: ObjectTypeVisibility.VISIBLE,
      description: 'Markdown notes',
    })
    expect(metadata.displayName).toBe('Notebook')
    expect(metadata.iconName).toBe('notebook')
    expect(metadata.visibility).toBe(ObjectTypeVisibility.VISIBLE)
    expect(metadata.description).toBe('Markdown notes')
  })

  it('ObjectTypeRegistration has typeId, registrationId, pluginId, and metadata fields', () => {
    const reg = ObjectTypeRegistration.create({
      typeId: 'test-type',
      registrationId: 7,
      pluginId: 'my-plugin',
      metadata: {
        displayName: 'Test Type',
        iconName: 'box',
        visibility: ObjectTypeVisibility.VISIBLE,
      },
    })
    expect(reg.typeId).toBe('test-type')
    expect(reg.registrationId).toBe(7)
    expect(reg.pluginId).toBe('my-plugin')
    expect(reg.metadata?.displayName).toBe('Test Type')
  })

  it('RegisterObjectTypeRequest has typeId, pluginId, and metadata fields', () => {
    const req = RegisterObjectTypeRequest.create({
      typeId: 'notes/notebook',
      pluginId: 'notes-plugin',
      metadata: {
        displayName: 'Notebook',
        iconName: 'notebook',
        visibility: ObjectTypeVisibility.VISIBLE,
        description: 'Markdown notes',
      },
    })
    expect(req.typeId).toBe('notes/notebook')
    expect(req.pluginId).toBe('notes-plugin')
    expect(req.metadata?.description).toBe('Markdown notes')
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
      typeId: 'notes/notebook',
      registrationId: 42,
      pluginId: 'notes-plugin',
      metadata: {
        displayName: 'Notebook',
        iconName: 'notebook',
        visibility: ObjectTypeVisibility.VISIBLE,
        description: 'Markdown notes',
      },
    })
    const bytes = ObjectTypeRegistration.toBinary(original)
    const decoded = ObjectTypeRegistration.fromBinary(bytes)
    expect(decoded.typeId).toBe('notes/notebook')
    expect(decoded.registrationId).toBe(42)
    expect(decoded.pluginId).toBe('notes-plugin')
    expect(decoded.metadata?.displayName).toBe('Notebook')
    expect(decoded.metadata?.visibility).toBe(ObjectTypeVisibility.VISIBLE)
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
  it('InvokeObjectTypeRequest has typeId, objectKey, attachedEngineResourceId', () => {
    const req = InvokeObjectTypeRequest.create({
      typeId: 'notes/notebook',
      objectKey: 'obj-123',
      attachedEngineResourceId: 5,
    })
    expect(req.typeId).toBe('notes/notebook')
    expect(req.objectKey).toBe('obj-123')
    expect(req.attachedEngineResourceId).toBe(5)
  })

  it('InvokeObjectTypeResponse has resourceId', () => {
    const resp = InvokeObjectTypeResponse.create({ resourceId: 10 })
    expect(resp.resourceId).toBe(10)
  })

  it('InvokeObjectTypeRequest round-trip serialization', () => {
    const original = InvokeObjectTypeRequest.create({
      typeId: 'canvas/board',
      objectKey: 'world-obj-456',
      attachedEngineResourceId: 88,
    })
    const bytes = InvokeObjectTypeRequest.toBinary(original)
    const decoded = InvokeObjectTypeRequest.fromBinary(bytes)
    expect(decoded.typeId).toBe('canvas/board')
    expect(decoded.objectKey).toBe('world-obj-456')
    expect(decoded.attachedEngineResourceId).toBe(88)
  })
})
