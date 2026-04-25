import { describe, expect, it } from 'vitest'

import {
  getPublicQuickstartOptions,
  getQuickstartOption,
  getVisibleQuickstartOptions,
  isQuickstartOptionPublic,
  isQuickstartOptionVisible,
} from './options.js'

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
    expect(isQuickstartOptionVisible(getQuickstartOption('forge'), true)).toBe(
      true,
    )
  })

  it('keeps hidden and path-based quickstarts out of public prerender pages', () => {
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
  })

  it('derives release-visible quickstart inventories from the same policy', () => {
    expect(
      getVisibleQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['account', 'pair', 'space', 'drive', 'git', 'canvas'])
    expect(
      getPublicQuickstartOptions(false).map((option) => option.id),
    ).toEqual(['space', 'drive', 'git', 'canvas'])
  })
})
