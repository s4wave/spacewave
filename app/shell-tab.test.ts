import { describe, expect, it } from 'vitest'

import { getTabNameFromPath } from './shell-tab.js'

describe('getTabNameFromPath', () => {
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
