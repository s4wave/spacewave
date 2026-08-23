import { describe, expect, it } from 'vitest'

import { spaceRouteCanRenderBody } from './spaceMountStage.js'

describe('spaceRouteCanRenderBody', () => {
  it('waits for inventory state on the space root', () => {
    expect(spaceRouteCanRenderBody(true, true, true, false, '')).toBe(false)
    expect(spaceRouteCanRenderBody(true, true, true, true, '')).toBe(true)
  })

  it('allows deep object routes once the world is mounted', () => {
    expect(spaceRouteCanRenderBody(true, true, true, false, 'files')).toBe(true)
    expect(spaceRouteCanRenderBody(true, true, true, false, 'files')).toBe(true)
  })
})
