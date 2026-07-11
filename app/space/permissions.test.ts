import { describe, expect, it } from 'vitest'

import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'

import { canDeleteSpaceObject, canRenameSpace } from './permissions.js'

describe('canRenameSpace', () => {
  it('allows rename for local spaces', () => {
    expect(canRenameSpace('local', false)).toBe(true)
  })

  it('allows rename for cloud spaces with manage permission', () => {
    expect(canRenameSpace('spacewave', true)).toBe(true)
  })

  it('blocks rename for cloud viewers without manage permission', () => {
    expect(canRenameSpace('spacewave', false)).toBe(false)
  })

  it('blocks rename for unsupported providers', () => {
    expect(canRenameSpace('other', true)).toBe(false)
  })
})

describe('canDeleteSpaceObject', () => {
  it('allows object deletion in local spaces', () => {
    expect(canDeleteSpaceObject('local', undefined)).toBe(true)
  })

  it.each([
    SOParticipantRole.SOParticipantRole_WRITER,
    SOParticipantRole.SOParticipantRole_VALIDATOR,
    SOParticipantRole.SOParticipantRole_OWNER,
  ])('allows cloud roles that can apply object mutations', (role) => {
    expect(canDeleteSpaceObject('spacewave', role)).toBe(true)
  })

  it.each([
    undefined,
    SOParticipantRole.SOParticipantRole_UNKNOWN,
    SOParticipantRole.SOParticipantRole_READER,
  ])('blocks cloud roles that cannot apply object mutations', (role) => {
    expect(canDeleteSpaceObject('spacewave', role)).toBe(false)
  })

  it('blocks unsupported providers', () => {
    expect(
      canDeleteSpaceObject('other', SOParticipantRole.SOParticipantRole_OWNER),
    ).toBe(false)
  })
})
