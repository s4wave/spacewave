import { describe, expect, it } from 'vitest'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  areExperimentalCreatorsEnabled,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import {
  getPublicQuickstartOptions,
  getQuickstartOption,
  getVisibleQuickstartOptions,
  isQuickstartOptionPublic,
  isQuickstartOptionVisible,
} from './options.js'
import { afterEach } from 'vitest'

afterEach(() => {
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('quickstart options', () => {
  it('keeps supported quickstarts visible in release', () => {
    expect(isQuickstartOptionVisible(getQuickstartOption('drive'), false)).toBe(
      true,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('space'), false)).toBe(
      true,
    )
    expect(
      isQuickstartOptionVisible(getQuickstartOption('canvas'), false),
    ).toBe(true)
    expect(isQuickstartOptionVisible(getQuickstartOption('git'), false)).toBe(
      true,
    )
  })

  it('hides experimental quickstarts in release and keeps them in dev', () => {
    expect(
      isQuickstartOptionVisible(getQuickstartOption('notebook'), false),
    ).toBe(false)
    expect(isQuickstartOptionVisible(getQuickstartOption('v86'), false)).toBe(
      false,
    )
    expect(
      isQuickstartOptionVisible(getQuickstartOption('device'), false),
    ).toBe(false)
    expect(isQuickstartOptionVisible(getQuickstartOption('device'), true)).toBe(
      true,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('forge'), true)).toBe(
      true,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('kv'), false)).toBe(
      false,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('kv'), true)).toBe(
      true,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('sql'), false)).toBe(
      false,
    )
    expect(isQuickstartOptionVisible(getQuickstartOption('sql'), true)).toBe(
      true,
    )
  })

  it('keeps hidden, dynamic, and path-based quickstarts out of public prerender pages', () => {
    expect(isQuickstartOptionPublic(getQuickstartOption('drive'), false)).toBe(
      true,
    )
    expect(
      isQuickstartOptionPublic(getQuickstartOption('account'), false),
    ).toBe(false)
    expect(isQuickstartOptionPublic(getQuickstartOption('local'), false)).toBe(
      false,
    )
    expect(
      isQuickstartOptionPublic(getQuickstartOption('notebook'), false),
    ).toBe(false)
    expect(
      isQuickstartOptionPublic(
        {
          ...getQuickstartOption('drive'),
          id: 'glados-workspace',
          dynamic: true,
        },
        false,
      ),
    ).toBe(false)
  })

  it('derives release-visible quickstart inventories from the same policy', () => {
    expect(
      getVisibleQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['account', 'pair', 'space', 'drive', 'git', 'canvas'])
    expect(
      getPublicQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['space', 'drive', 'git', 'canvas'])
    expect(
      getVisibleQuickstartOptions(true).map((option) => option.id),
    ).toContain('device')
  })

  it('keeps drive quickstart copy aligned without changing release ordering', () => {
    const drive = getQuickstartOption('drive')

    expect(drive.description).toBe(
      'Private browser files with offline work and device sync',
    )
    expect(drive.seoDescription).toContain('offline file browsing')
    expect(drive.seoDescription).toContain(
      'private sync through your own devices',
    )
    expect(
      getVisibleQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['account', 'pair', 'space', 'drive', 'git', 'canvas'])
  })

  it('lets release runtime visibility include experimental quickstarts without changing public release pages', () => {
    setExperimentalCreatorsEnabled(true)
    const runtimeEnabled = areExperimentalCreatorsEnabled(false)

    expect(runtimeEnabled).toBe(true)
    expect(
      getVisibleQuickstartOptions(runtimeEnabled).map((option) => option.id),
    ).toContain('device')
    expect(
      getPublicQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['space', 'drive', 'git', 'canvas'])
  })
})
