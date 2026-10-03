import { afterEach, describe, expect, it } from 'vitest'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  areExperimentalCreatorsEnabled,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import { getVisibleQuickstartOptions } from './options.js'
import {
  dynamicQuickstartRegistrationToOption,
  getDynamicQuickstartIcon,
  mergeQuickstartOptions,
} from './dynamic-options.js'

afterEach(() => {
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('dynamic quickstart options', () => {
  it('converts plugin registrations to app-only options', () => {
    const option = dynamicQuickstartRegistrationToOption(
      {
        quickstartId: 'gizmo-workspace',
        registrationId: 1,
        pluginId: 'gizmo-web',
        name: 'Gizmo Workspace',
        description: 'Operator workspace',
        category: 'tools',
        iconName: 'bot',
        spaceName: 'Gizmo Workspace',
        requiredPluginIds: ['gizmo-core', 'gizmo-web'],
      },
      false,
    )
    expect(option?.id).toBe('gizmo-workspace')
    expect(option?.dynamic).toBe(true)
    expect(option?.pluginId).toBe('gizmo-web')
    expect(option?.requiredPluginIds).toEqual(['gizmo-core', 'gizmo-web'])
    expect(option?.path).toBeUndefined()
  })

  it('hides hidden and release-invisible experimental registrations', () => {
    expect(
      dynamicQuickstartRegistrationToOption(
        {
          quickstartId: 'hidden',
          pluginId: 'plugin',
          name: 'Hidden',
          description: 'Hidden workspace',
          category: 'tools',
          hidden: true,
        },
        true,
      ),
    ).toBeNull()
    expect(
      dynamicQuickstartRegistrationToOption(
        {
          quickstartId: 'experimental',
          pluginId: 'plugin',
          name: 'Experimental',
          description: 'Experimental workspace',
          category: 'tools',
          experimental: true,
        },
        false,
      ),
    ).toBeNull()
    expect(
      dynamicQuickstartRegistrationToOption(
        {
          quickstartId: 'experimental',
          pluginId: 'plugin',
          name: 'Experimental',
          description: 'Experimental workspace',
          category: 'tools',
          experimental: true,
        },
        true,
      )?.id,
    ).toBe('experimental')
  })

  it('keeps static options first and lets static ids win', () => {
    const merged = mergeQuickstartOptions(
      getVisibleQuickstartOptions(false),
      [
        {
          quickstartId: 'drive',
          pluginId: 'plugin',
          name: 'Plugin Drive',
          description: 'Should not replace static Drive',
          category: 'tools',
        },
        {
          quickstartId: 'gizmo-workspace',
          pluginId: 'gizmo-web',
          name: 'Gizmo Workspace',
          description: 'Operator workspace',
          category: 'tools',
        },
      ],
      false,
    )
    expect(merged.map((option) => option.id)).toEqual([
      'account',
      'pair',
      'space',
      'drive',
      'git',
      'canvas',
      'gizmo-workspace',
    ])
    expect(merged.find((option) => option.id === 'drive')?.name).toBe(
      'Create a Drive',
    )
  })

  it('uses release runtime visibility for experimental dynamic registrations', () => {
    setExperimentalCreatorsEnabled(true)
    const merged = mergeQuickstartOptions(
      getVisibleQuickstartOptions(false),
      [
        {
          quickstartId: 'experimental',
          pluginId: 'plugin',
          name: 'Experimental',
          description: 'Experimental workspace',
          category: 'tools',
          experimental: true,
        },
      ],
      areExperimentalCreatorsEnabled(false),
    )

    expect(merged.map((option) => option.id)).toContain('experimental')
  })

  it('falls back to the box icon for unknown dynamic icon names', () => {
    expect(getDynamicQuickstartIcon('missing')).toBe(getDynamicQuickstartIcon())
  })
})
