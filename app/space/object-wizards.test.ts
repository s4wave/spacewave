import { describe, expect, it } from 'vitest'
import type { ObjectWizard } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import {
  isObjectWizardVisible,
  normalizeObjectWizards,
} from './object-wizards.js'

describe('normalizeObjectWizards', () => {
  it('filters non-creatable entries and deduplicates by type ID', () => {
    const wizards: ObjectWizard[] = [
      {
        typeId: 'bldr/web/fetch/service',
        displayName: 'spacewave-core',
        createOpId: 'Developer',
        keyPrefix: '/b/pkg/fetch',
      },
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
      },
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        category: 'Layout',
        iconName: 'LuLayoutGrid',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
      },
    ]

    expect(normalizeObjectWizards(wizards)).toEqual([
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        category: 'Layout',
        iconName: 'LuLayoutGrid',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
      },
    ])
  })

  it('keeps persistent wizards without a local create-op builder', () => {
    const wizards: ObjectWizard[] = [
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        persistent: true,
        wizardTypeId: 'wizard/forge/task',
      },
    ]

    expect(normalizeObjectWizards(wizards)).toEqual(wizards)
  })

  it('filters experimental wizards in release but keeps them in dev', () => {
    const wizards: ObjectWizard[] = [
      {
        typeId: 'git/repo',
        displayName: 'Git Repository',
        persistent: true,
        wizardTypeId: 'wizard/git/repo',
      },
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        persistent: true,
        wizardTypeId: 'wizard/forge/task',
        experimental: true,
      },
    ]

    expect(isObjectWizardVisible(wizards[0], false)).toBe(true)
    expect(isObjectWizardVisible(wizards[1], false)).toBe(false)
    expect(normalizeObjectWizards(wizards, false)).toEqual([wizards[0]])
    expect(normalizeObjectWizards(wizards, true)).toEqual(wizards)
  })
})
