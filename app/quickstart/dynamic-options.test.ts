import { describe, expect, it } from 'vitest'

import { getVisibleQuickstartOptions } from './options.js'
import {
  dynamicQuickstartRegistrationToOption,
  getDynamicQuickstartIcon,
  mergeQuickstartOptions,
} from './dynamic-options.js'

describe('dynamic quickstart options', () => {
  it('converts plugin registrations to app-only options', () => {
    const option = dynamicQuickstartRegistrationToOption(
      {
        quickstartId: 'glados-workspace',
        registrationId: 1,
        pluginId: 'glados-web',
        name: 'Glados Workspace',
        description: 'Operator workspace',
        category: 'tools',
        iconName: 'bot',
        spaceName: 'Glados Workspace',
        requiredPluginIds: ['glados-core', 'glados-web'],
      },
      false,
    )
    expect(option?.id).toBe('glados-workspace')
    expect(option?.dynamic).toBe(true)
    expect(option?.pluginId).toBe('glados-web')
    expect(option?.requiredPluginIds).toEqual(['glados-core', 'glados-web'])
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
          quickstartId: 'glados-workspace',
          pluginId: 'glados-web',
          name: 'Glados Workspace',
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
      'glados-workspace',
    ])
    expect(merged.find((option) => option.id === 'drive')?.name).toBe(
      'Create a Drive',
    )
  })

  it('falls back to the box icon for unknown dynamic icon names', () => {
    expect(getDynamicQuickstartIcon('missing')).toBe(getDynamicQuickstartIcon())
  })
})
