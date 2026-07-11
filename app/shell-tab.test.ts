import { describe, expect, it } from 'vitest'

import { getTabNameFromPath } from './shell-tab.js'

describe('getTabNameFromPath', () => {
  it('labels docs routes as Docs', () => {
    expect(getTabNameFromPath('/docs')).toBe('Docs')
    expect(getTabNameFromPath('/docs/developer/install')).toBe('Docs')
  })
  it('labels pairing routes with a descriptive tab name', () => {
    expect(getTabNameFromPath('/pair')).toBe('Pair device')
    expect(getTabNameFromPath('/pair/ABCD1234')).toBe('Pair device')
    expect(getTabNameFromPath('/u/1/pair')).toBe('Pair device')
  })

  it('labels link-device setup routes with a descriptive tab name', () => {
    expect(getTabNameFromPath('/setup/link-device')).toBe('Link device')
    expect(getTabNameFromPath('/u/1/setup/link-device')).toBe('Link device')
  })

  it('uses built-in object labels for space object routes', () => {
    expect(getTabNameFromPath('/u/1/so/space/-/unixfs')).toBe('Files')
    expect(getTabNameFromPath('/u/1/so/space/-/object-layout')).toBe('Layout')
  })

  it('humanizes generated object keys instead of showing the route', () => {
    expect(
      getTabNameFromPath('/u/1/so/space/-/glados/bootstrap/llm-session'),
    ).toBe('Llm Session')
  })
})
