import { describe, expect, it } from 'vitest'

import {
  spaceRouteCanRenderBody,
  spaceRouteShouldMountContents,
} from './spaceMountStage.js'

describe('spaceRouteCanRenderBody', () => {
  it('waits for inventory state on the space root', () => {
    expect(spaceRouteCanRenderBody(true, true, true, true, false, '')).toBe(
      false,
    )
    expect(spaceRouteCanRenderBody(true, true, true, true, true, '')).toBe(true)
  })

  it('allows deep object routes once the world is mounted', () => {
    expect(
      spaceRouteCanRenderBody(true, true, true, false, false, 'files'),
    ).toBe(true)
    expect(
      spaceRouteCanRenderBody(true, true, true, true, false, 'files'),
    ).toBe(true)
  })
})

describe('spaceRouteShouldMountContents', () => {
  it('keeps the space root dependent on the contents controller', () => {
    expect(spaceRouteShouldMountContents('', false)).toBe(true)
    expect(spaceRouteShouldMountContents(undefined, false)).toBe(true)
  })

  it('defers contents mounting for deep routes until inventory state is ready', () => {
    expect(spaceRouteShouldMountContents('files', false)).toBe(false)
    expect(spaceRouteShouldMountContents('files', true)).toBe(true)
  })
})
