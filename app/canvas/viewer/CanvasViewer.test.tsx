import { describe, expect, it } from 'vitest'

import { isCanvasInsertableObject } from './object-picker.js'

describe('isCanvasInsertableObject', () => {
  it('hides the reserved space settings object', () => {
    expect(
      isCanvasInsertableObject('settings', 'space/settings', 'canvas-1'),
    ).toBe(false)
  })

  it('keeps regular objects available for insertion', () => {
    expect(
      isCanvasInsertableObject(
        'object-layout/main',
        'alpha/object-layout',
        'canvas-1',
      ),
    ).toBe(true)
  })
})
