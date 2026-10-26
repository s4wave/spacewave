import { describe, expect, it } from 'vitest'

import { AlarmState, type Alarm } from '@s4wave/sdk/alarm/alarm.pb.js'

import { alarmStatus, chipText, holdText, linkedAlarms } from './alarm-view.js'
import { memberName } from './AlarmViewer.js'

const min = 60_000
const t0 = 1_793_000_000_000

describe('alarmStatus', () => {
  const who = (p: string) => (p === 'peer-a' ? 'cjs' : p)

  it('describes each state', () => {
    const holding: Alarm = {
      state: AlarmState.HOLDING,
      sinceMs: BigInt(t0),
      holdMs: BigInt(2 * min),
    }
    expect(alarmStatus(holding, t0 + min, who)).toMatchObject({
      word: 'Holding',
      tone: 'holding',
      sounding: true,
    })
    expect(alarmStatus(holding, t0 + min, who).line).toMatch(/\(in 60s\)\.$/)

    const firing: Alarm = { state: AlarmState.FIRING, firedMs: BigInt(t0) }
    expect(alarmStatus(firing, t0, who)).toMatchObject({
      word: 'Firing',
      tone: 'alert',
    })

    const acked: Alarm = {
      state: AlarmState.ACKNOWLEDGED,
      firedMs: BigInt(t0),
      acknowledgedMs: BigInt(t0 + min),
      acknowledgedBy: 'peer-a',
    }
    expect(alarmStatus(acked, t0, who).line).toContain('Acknowledged by cjs')

    expect(alarmStatus({}, t0, who)).toMatchObject({
      word: 'Clear',
      line: 'Clear.',
      sounding: false,
    })
    expect(alarmStatus({ clearedMs: BigInt(t0) }, t0, who).line).toMatch(
      /^Clear since /,
    )
  })

  it('writes the hold and the chip', () => {
    expect(holdText({ holdMs: BigInt(2 * min) })).toBe(
      'Fires after 2m of this condition.',
    )
    expect(holdText({})).toBe('Fires as soon as this condition holds.')
    expect(
      chipText({ state: AlarmState.FIRING, firedMs: BigInt(t0) }, t0 + 4 * min),
    ).toBe('Firing 4m')
  })
})

describe('linkedAlarms', () => {
  it('follows reads edges to the roots through other Alarms', () => {
    const reads = new Map([
      ['alarm/direct', ['dash']],
      ['alarm/target', ['target/forge']],
      ['alarm/meta', ['alarm/target']],
      ['alarm/meta2', ['alarm/meta']],
      ['alarm/object', ['some/object']],
      ['alarm/none', []],
    ])
    expect(
      [...linkedAlarms(reads, new Set(['dash', 'target/forge']))].sort(),
    ).toEqual(['alarm/direct', 'alarm/meta', 'alarm/meta2', 'alarm/target'])
  })
})

describe('memberName', () => {
  it('names this member, a participant, or a short peer', () => {
    const state = {
      participantInfo: [{ entityId: 'alice', peerIds: ['peer-b'] }],
    }
    expect(memberName(state, 'peer-me', 'peer-me')).toBe('you')
    expect(memberName(state, 'peer-b', 'peer-me')).toBe('alice')
    expect(memberName(null, '12D3KooWAbcdefXYZ', 'peer-me')).toMatch(/^peer /)
  })
})
