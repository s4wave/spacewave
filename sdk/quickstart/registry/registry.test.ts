import { describe, it, expect } from 'vitest'

import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

import {
  ExecuteQuickstartRequest,
  ExecuteQuickstartResponse,
  ListQuickstartsResponse,
  QuickstartRegistration,
  RegisterQuickstartRequest,
  RegisterQuickstartResponse,
  SeedQuickstartRequest,
  SeedQuickstartResponse,
  WatchQuickstartsResponse,
} from './registry.pb.js'
import { QuickstartRegistry } from './registry.js'

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

describe('QuickstartRegistry proto types', () => {
  it('QuickstartRegistration carries plugin-owned app metadata', () => {
    const reg = QuickstartRegistration.create({
      quickstartId: 'glados-workspace',
      registrationId: 7,
      pluginId: 'glados-web',
      name: 'Glados Workspace',
      description: 'Operator workspace',
      category: 'tools',
      iconName: 'bot',
      spaceName: 'Glados Workspace',
      requiredPluginIds: ['glados-core', 'glados-web'],
    })
    expect(reg.quickstartId).toBe('glados-workspace')
    expect(reg.registrationId).toBe(7)
    expect(reg.pluginId).toBe('glados-web')
    expect(reg.name).toBe('Glados Workspace')
    expect(reg.requiredPluginIds).toEqual(['glados-core', 'glados-web'])
  })

  it('RegisterQuickstartRequest has registration metadata', () => {
    const req = RegisterQuickstartRequest.create({
      registration: {
        quickstartId: 'glados-workspace',
        pluginId: 'glados-web',
        name: 'Glados Workspace',
        description: 'Operator workspace',
        category: 'tools',
      },
    })
    expect(req.registration?.quickstartId).toBe('glados-workspace')
    expect(req.registration?.pluginId).toBe('glados-web')
  })

  it('RegisterQuickstartResponse has resourceId', () => {
    const resp = RegisterQuickstartResponse.create({ resourceId: 99 })
    expect(resp.resourceId).toBe(99)
  })

  it('execution messages carry resource ids and routing hints', () => {
    const executeReq = ExecuteQuickstartRequest.create({
      quickstartId: 'glados-workspace',
      spaceResourceId: 42,
    })
    const executeResp = ExecuteQuickstartResponse.create({
      indexPath: 'glados/org-chart',
      pluginIds: ['glados-core', 'glados-web'],
    })
    const seedReq = SeedQuickstartRequest.create({
      quickstartId: 'glados-workspace',
      attachedEngineResourceId: 77,
    })
    const seedResp = SeedQuickstartResponse.create({
      indexPath: 'glados/org-chart',
      pluginIds: ['glados-web'],
    })
    expect(executeReq.spaceResourceId).toBe(42)
    expect(executeResp.pluginIds).toEqual(['glados-core', 'glados-web'])
    expect(seedReq.attachedEngineResourceId).toBe(77)
    expect(seedResp.indexPath).toBe('glados/org-chart')
  })

  it('ListQuickstartsResponse and WatchQuickstartsResponse carry registrations', () => {
    const registrations = [
      { quickstartId: 'alpha', registrationId: 1, pluginId: 'plugin-a' },
      { quickstartId: 'zeta', registrationId: 2, pluginId: 'plugin-b' },
    ]
    const list = ListQuickstartsResponse.create({ registrations })
    const watch = WatchQuickstartsResponse.create({ registrations })
    expect(list.registrations).toHaveLength(2)
    expect(watch.registrations?.[1].pluginId).toBe('plugin-b')
  })

  it('QuickstartRegistration round-trips through binary serialization', () => {
    const original = QuickstartRegistration.create({
      quickstartId: 'glados-workspace',
      registrationId: 42,
      pluginId: 'glados-web',
      name: 'Glados Workspace',
      description: 'Operator workspace',
      category: 'tools',
      requiredPluginIds: ['glados-core', 'glados-web'],
    })
    const bytes = QuickstartRegistration.toBinary(original)
    const decoded = QuickstartRegistration.fromBinary(bytes)
    expect(decoded.quickstartId).toBe('glados-workspace')
    expect(decoded.registrationId).toBe(42)
    expect(decoded.requiredPluginIds).toEqual(['glados-core', 'glados-web'])
  })
})

describe('QuickstartRegistry SDK class', () => {
  it('constructs with mock ClientResourceRef', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(registry).toBeDefined()
    expect(registry.id).toBe(42)
  })

  it('extends Resource base class', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(registry).toBeInstanceOf(Resource)
  })

  it('has registerQuickstart method', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(typeof registry.registerQuickstart).toBe('function')
  })

  it('has listQuickstarts method', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(typeof registry.listQuickstarts).toBe('function')
  })

  it('has watchQuickstarts method', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(typeof registry.watchQuickstarts).toBe('function')
  })

  it('has executeQuickstart method', () => {
    const ref = createMockRef()
    const registry = new QuickstartRegistry(ref)
    expect(typeof registry.executeQuickstart).toBe('function')
  })
})
