import { describe, it, expect } from 'vitest'
import {
  WorldOpRegistration,
  RegisterWorldOpRequest,
  RegisterWorldOpResponse,
  WatchWorldOpsResponse,
  ApplyWorldOpRequest,
  ApplyWorldObjectOpRequest,
  ValidateOpRequest,
  ValidateOpResponse,
} from './registry.pb.js'
import { WorldOpRegistry } from './registry.js'
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

describe('WorldOpRegistry proto types', () => {
  it('WorldOpRegistration has operationTypeId, registrationId, pluginId fields', () => {
    const reg = WorldOpRegistration.create({
      operationTypeId: 'create-object',
      registrationId: 3,
      pluginId: 'core-plugin',
    })
    expect(reg.operationTypeId).toBe('create-object')
    expect(reg.registrationId).toBe(3)
    expect(reg.pluginId).toBe('core-plugin')
  })

  it('RegisterWorldOpRequest has operationTypeId and pluginId fields', () => {
    const req = RegisterWorldOpRequest.create({
      operationTypeId: 'delete-object',
      pluginId: 'mgmt-plugin',
    })
    expect(req.operationTypeId).toBe('delete-object')
    expect(req.pluginId).toBe('mgmt-plugin')
  })

  it('RegisterWorldOpResponse has resourceId field', () => {
    const resp = RegisterWorldOpResponse.create({ resourceId: 55 })
    expect(resp.resourceId).toBe(55)
  })

  it('WatchWorldOpsResponse has registrations array', () => {
    const resp = WatchWorldOpsResponse.create({
      registrations: [
        { operationTypeId: 'op-a', registrationId: 1, pluginId: 'plugin-a' },
        { operationTypeId: 'op-b', registrationId: 2, pluginId: 'plugin-b' },
      ],
    })
    expect(resp.registrations).toHaveLength(2)
    expect(resp.registrations![0].operationTypeId).toBe('op-a')
    expect(resp.registrations![1].pluginId).toBe('plugin-b')
  })

  it('WorldOpRegistration round-trip serialization', () => {
    const original = WorldOpRegistration.create({
      operationTypeId: 'move-object',
      registrationId: 17,
      pluginId: 'world-plugin',
    })
    const bytes = WorldOpRegistration.toBinary(original)
    const decoded = WorldOpRegistration.fromBinary(bytes)
    expect(decoded.operationTypeId).toBe('move-object')
    expect(decoded.registrationId).toBe(17)
    expect(decoded.pluginId).toBe('world-plugin')
  })
})

describe('WorldOpRegistry SDK class', () => {
  it('constructs with mock ClientResourceRef', () => {
    const ref = createMockRef()
    const registry = new WorldOpRegistry(ref)
    expect(registry).toBeDefined()
    expect(registry.id).toBe(42)
  })

  it('extends Resource base class', () => {
    const ref = createMockRef()
    const registry = new WorldOpRegistry(ref)
    expect(registry).toBeInstanceOf(Resource)
  })

  it('has registerWorldOp method', () => {
    const ref = createMockRef()
    const registry = new WorldOpRegistry(ref)
    expect(typeof registry.registerWorldOp).toBe('function')
  })

  it('has watchWorldOps method', () => {
    const ref = createMockRef()
    const registry = new WorldOpRegistry(ref)
    expect(typeof registry.watchWorldOps).toBe('function')
  })
})

describe('WorldOpHandlerService proto types', () => {
  it('ApplyWorldOpRequest has operationTypeId, opData, engineResourceId', () => {
    const data = new Uint8Array([1, 2, 3])
    const req = ApplyWorldOpRequest.create({
      operationTypeId: 'create-object',
      opData: data,
      engineResourceId: 12,
    })
    expect(req.operationTypeId).toBe('create-object')
    expect(req.opData).toEqual(data)
    expect(req.engineResourceId).toBe(12)
  })

  it('ApplyWorldObjectOpRequest has operationTypeId, opData, objectKey, engineResourceId', () => {
    const data = new Uint8Array([4, 5, 6])
    const req = ApplyWorldObjectOpRequest.create({
      operationTypeId: 'update-field',
      opData: data,
      objectKey: 'obj-789',
      engineResourceId: 20,
    })
    expect(req.operationTypeId).toBe('update-field')
    expect(req.opData).toEqual(data)
    expect(req.objectKey).toBe('obj-789')
    expect(req.engineResourceId).toBe(20)
  })

  it('ValidateOpRequest has operationTypeId, opData', () => {
    const data = new Uint8Array([7, 8])
    const req = ValidateOpRequest.create({
      operationTypeId: 'rename-object',
      opData: data,
    })
    expect(req.operationTypeId).toBe('rename-object')
    expect(req.opData).toEqual(data)
  })

  it('ValidateOpResponse has error field', () => {
    const resp = ValidateOpResponse.create({ error: 'invalid name' })
    expect(resp.error).toBe('invalid name')
  })

  it('ValidateOpResponse with empty error indicates success', () => {
    const resp = ValidateOpResponse.create({})
    expect(resp.error).toBeUndefined()
  })

  it('ApplyWorldOpRequest round-trip serialization', () => {
    const data = new Uint8Array([10, 20, 30, 40])
    const original = ApplyWorldOpRequest.create({
      operationTypeId: 'batch-update',
      opData: data,
      engineResourceId: 77,
    })
    const bytes = ApplyWorldOpRequest.toBinary(original)
    const decoded = ApplyWorldOpRequest.fromBinary(bytes)
    expect(decoded.operationTypeId).toBe('batch-update')
    expect(decoded.opData).toEqual(data)
    expect(decoded.engineResourceId).toBe(77)
  })
})
