import { describe, it, expect } from 'vitest'
import { Model, IJsonModel } from '@aptre/flex-layout'

import {
  hasGridLayout,
  encodeGridLayout,
  decodeGridLayout,
  getSelectedTabId,
  getActiveTabsetId,
  getTabIdsFromModel,
  SHELL_GRID_BASE_MODEL,
} from './shell-grid-utils.js'

// Helper to create a simple single-tabset model
function createSingleTabsetModel(tabCount = 2): IJsonModel {
  return {
    ...SHELL_GRID_BASE_MODEL,
    layout: {
      type: 'row',
      weight: 100,
      children: [
        {
          type: 'tabset',
          id: 'tabset-1',
          weight: 100,
          selected: 0,
          children: Array.from({ length: tabCount }, (_, i) => ({
            type: 'tab' as const,
            id: `tab-${i + 1}`,
            name: `Tab ${i + 1}`,
            component: 'empty',
          })),
        },
      ],
    },
  }
}

// Helper to create a model with horizontal split (two tabsets side by side)
function createHorizontalSplitModel(): IJsonModel {
  return {
    ...SHELL_GRID_BASE_MODEL,
    layout: {
      type: 'row',
      weight: 100,
      children: [
        {
          type: 'tabset',
          id: 'tabset-1',
          weight: 50,
          selected: 0,
          children: [
            {
              type: 'tab' as const,
              id: 'tab-1',
              name: 'Tab 1',
              component: 'empty',
            },
          ],
        },
        {
          type: 'tabset',
          id: 'tabset-2',
          weight: 50,
          selected: 0,
          children: [
            {
              type: 'tab' as const,
              id: 'tab-2',
              name: 'Tab 2',
              component: 'empty',
            },
          ],
        },
      ],
    },
  }
}

// Helper to create a model with vertical split (nested rows)
function createVerticalSplitModel(): IJsonModel {
  return {
    ...SHELL_GRID_BASE_MODEL,
    layout: {
      type: 'row',
      weight: 100,
      children: [
        {
          type: 'row',
          weight: 50,
          children: [
            {
              type: 'tabset',
              id: 'tabset-1',
              weight: 100,
              selected: 0,
              children: [
                {
                  type: 'tab' as const,
                  id: 'tab-1',
                  name: 'Tab 1',
                  component: 'empty',
                },
              ],
            },
          ],
        },
        {
          type: 'row',
          weight: 50,
          children: [
            {
              type: 'tabset',
              id: 'tabset-2',
              weight: 100,
              selected: 0,
              children: [
                {
                  type: 'tab' as const,
                  id: 'tab-2',
                  name: 'Tab 2',
                  component: 'empty',
                },
              ],
            },
          ],
        },
      ],
    },
  }
}

describe('shell-grid-utils', () => {
  describe('hasGridLayout', () => {
    it('returns false for single tabset with one tab', () => {
      const jsonModel = createSingleTabsetModel(1)
      const model = Model.fromJson(jsonModel)
      expect(hasGridLayout(model)).toBe(false)
    })

    it('returns false for single tabset with multiple tabs', () => {
      const jsonModel = createSingleTabsetModel(3)
      const model = Model.fromJson(jsonModel)
      expect(hasGridLayout(model)).toBe(false)
    })

    it('returns true for horizontal split (two tabsets)', () => {
      const jsonModel = createHorizontalSplitModel()
      const model = Model.fromJson(jsonModel)
      expect(hasGridLayout(model)).toBe(true)
    })

    it('returns true for vertical split (nested rows)', () => {
      const jsonModel = createVerticalSplitModel()
      const model = Model.fromJson(jsonModel)
      expect(hasGridLayout(model)).toBe(true)
    })
  })

  describe('encodeGridLayout / decodeGridLayout', () => {
    it('encodes and decodes a simple model correctly', () => {
      const original = createSingleTabsetModel(2)
      const model = Model.fromJson(original)
      const encoded = encodeGridLayout(model)

      expect(typeof encoded).toBe('string')
      expect(encoded.length).toBeGreaterThan(0)
      // Should be URL-safe (no +, /, or =)
      expect(encoded).not.toMatch(/[+/=]/)

      const decoded = decodeGridLayout(encoded, SHELL_GRID_BASE_MODEL)
      expect(decoded).not.toBeNull()
      expect(decoded?.model.layout.type).toBe('row')
    })

    it('encodes and decodes a split model correctly', () => {
      const original = createHorizontalSplitModel()
      const model = Model.fromJson(original)
      const encoded = encodeGridLayout(model)
      const decoded = decodeGridLayout(encoded, SHELL_GRID_BASE_MODEL)

      expect(decoded).not.toBeNull()

      // Verify the split structure is preserved
      const decodedModel = Model.fromJson(decoded!.model)
      expect(hasGridLayout(decodedModel)).toBe(true)
    })

    it('returns null for invalid encoded data', () => {
      const result = decodeGridLayout(
        'invalid-base64-data!!!',
        SHELL_GRID_BASE_MODEL,
      )
      expect(result).toBeNull()
    })

    it('returns null for empty string', () => {
      const result = decodeGridLayout('', SHELL_GRID_BASE_MODEL)
      expect(result).toBeNull()
    })

    it('produces URL-safe base64', () => {
      const jsonModel = createHorizontalSplitModel()
      const model = Model.fromJson(jsonModel)
      const encoded = encodeGridLayout(model)

      // URL-safe base64 uses - instead of + and _ instead of /
      expect(encoded).not.toContain('+')
      expect(encoded).not.toContain('/')
      expect(encoded).not.toContain('=')

      // Should be usable in a URL without encoding
      const url = `https://example.com/#/g/${encoded}`
      expect(url).toBe(encodeURI(url))
    })
  })

  describe('getTabIdsFromModel', () => {
    it('returns all tab IDs from single tabset', () => {
      const jsonModel = createSingleTabsetModel(3)
      const model = Model.fromJson(jsonModel)
      const ids = getTabIdsFromModel(model)

      expect(ids).toHaveLength(3)
      expect(ids).toContain('tab-1')
      expect(ids).toContain('tab-2')
      expect(ids).toContain('tab-3')
    })

    it('returns all tab IDs from split model', () => {
      const jsonModel = createHorizontalSplitModel()
      const model = Model.fromJson(jsonModel)
      const ids = getTabIdsFromModel(model)

      expect(ids).toHaveLength(2)
      expect(ids).toContain('tab-1')
      expect(ids).toContain('tab-2')
    })
  })

  describe('getSelectedTabId', () => {
    it('returns selected tab ID from active tabset', () => {
      const jsonModel: IJsonModel = {
        ...SHELL_GRID_BASE_MODEL,
        layout: {
          type: 'row',
          weight: 100,
          children: [
            {
              type: 'tabset',
              id: 'tabset-1',
              weight: 100,
              selected: 1, // Second tab selected
              active: true,
              children: [
                { type: 'tab', id: 'tab-1', name: 'Tab 1', component: 'empty' },
                { type: 'tab', id: 'tab-2', name: 'Tab 2', component: 'empty' },
              ],
            },
          ],
        },
      }
      const model = Model.fromJson(jsonModel)
      const selectedId = getSelectedTabId(model)

      expect(selectedId).toBe('tab-2')
    })

    it('falls back to first tabset if none is active', () => {
      const jsonModel = createSingleTabsetModel(2)
      const model = Model.fromJson(jsonModel)
      const selectedId = getSelectedTabId(model)

      // Should return the selected tab from the first tabset
      expect(selectedId).toBe('tab-1')
    })
  })

  describe('getActiveTabsetId', () => {
    it('returns active tabset ID', () => {
      const jsonModel: IJsonModel = {
        ...SHELL_GRID_BASE_MODEL,
        layout: {
          type: 'row',
          weight: 100,
          children: [
            {
              type: 'tabset',
              id: 'tabset-1',
              weight: 50,
              children: [
                { type: 'tab', id: 'tab-1', name: 'Tab 1', component: 'empty' },
              ],
            },
            {
              type: 'tabset',
              id: 'tabset-2',
              weight: 50,
              active: true,
              children: [
                { type: 'tab', id: 'tab-2', name: 'Tab 2', component: 'empty' },
              ],
            },
          ],
        },
      }
      const model = Model.fromJson(jsonModel)
      const activeId = getActiveTabsetId(model)

      expect(activeId).toBe('tabset-2')
    })

    it('falls back to first tabset if none is active', () => {
      const jsonModel = createSingleTabsetModel(1)
      const model = Model.fromJson(jsonModel)
      const activeId = getActiveTabsetId(model)

      expect(activeId).toBe('tabset-1')
    })
  })

  describe('roundtrip encoding', () => {
    it('preserves tab structure through encode/decode cycle', () => {
      const original = createHorizontalSplitModel()
      const originalModel = Model.fromJson(original)
      const encoded = encodeGridLayout(originalModel)
      const decoded = decodeGridLayout(encoded, SHELL_GRID_BASE_MODEL)

      expect(decoded).not.toBeNull()

      const decodedModel = Model.fromJson(decoded!.model)

      // Both should have the same tab IDs
      const originalIds = getTabIdsFromModel(originalModel)
      const decodedIds = getTabIdsFromModel(decodedModel)

      expect(decodedIds).toEqual(originalIds)

      // Both should have grid layout
      expect(hasGridLayout(decodedModel)).toBe(hasGridLayout(originalModel))
    })

    it('preserves complex nested layout', () => {
      const original = createVerticalSplitModel()
      const originalModel = Model.fromJson(original)
      const encoded = encodeGridLayout(originalModel)
      const decoded = decodeGridLayout(encoded, SHELL_GRID_BASE_MODEL)

      expect(decoded).not.toBeNull()

      const decodedModel = Model.fromJson(decoded!.model)
      expect(hasGridLayout(decodedModel)).toBe(true)

      const ids = getTabIdsFromModel(decodedModel)
      expect(ids).toContain('tab-1')
      expect(ids).toContain('tab-2')
    })

    it('preserves local state through encode/decode cycle', () => {
      const jsonModel: IJsonModel = {
        ...SHELL_GRID_BASE_MODEL,
        layout: {
          type: 'row',
          weight: 100,
          children: [
            {
              type: 'tabset',
              id: 'tabset-1',
              weight: 50,
              selected: 1, // Second tab selected
              children: [
                { type: 'tab', id: 'tab-1', name: 'Tab 1', component: 'empty' },
                { type: 'tab', id: 'tab-2', name: 'Tab 2', component: 'empty' },
              ],
            },
            {
              type: 'tabset',
              id: 'tabset-2',
              weight: 50,
              active: true,
              selected: 0,
              children: [
                { type: 'tab', id: 'tab-3', name: 'Tab 3', component: 'empty' },
              ],
            },
          ],
        },
      }

      const model = Model.fromJson(jsonModel)
      const encoded = encodeGridLayout(model)
      const decoded = decodeGridLayout(encoded, SHELL_GRID_BASE_MODEL)

      expect(decoded).not.toBeNull()
      expect(decoded?.localState).toBeDefined()
      expect(decoded?.localState?.activeTabSetId).toBe('tabset-2')
      expect(decoded?.localState?.tabSetSelections?.['tabset-1']).toBe('tab-2')
      expect(decoded?.localState?.tabSetSelections?.['tabset-2']).toBe('tab-3')
    })
  })
})
