import { describe, expect, it } from 'vitest'
import {
  CommandSurface,
  type KeybindingOverrideSet as ProtoKeybindingOverrideSet,
} from '@s4wave/sdk/command/command.pb.js'
import {
  keybindingOverrideSetFromProto,
  keybindingOverrideSetToProto,
  mergeKeybindingOverridePartitions,
  migrateLegacyKeybindingOverrideSet,
  type KeybindingOverrideSet,
} from './keybinding-overrides.js'

const canonical = new Set(['spacewave.open'])
function legacy(
  overrides: ProtoKeybindingOverrideSet['overrides'] = [],
): ProtoKeybindingOverrideSet {
  return {
    version: 1,
    overrides,
    settings: { leaderCombo: 'Ctrl+Space' },
    webOverrides: [],
    tuiOverrides: [],
  }
}

describe('legacy keybinding migration', () => {
  it('is a no-op for v2 and preserves explicit TUI state', () => {
    const value: ProtoKeybindingOverrideSet = {
      version: 2,
      overrides: [],
      webOverrides: [],
      tuiOverrides: [
        {
          commandId: 'spacewave.open',
          bindings: [
            {
              id: 'tui',
              binding: { case: 'combo', value: { combo: 'q' } },
              surface: CommandSurface.TUI,
            },
          ],
        },
      ],
    }
    const result = migrateLegacyKeybindingOverrideSet(value, canonical)
    expect(result.required).toBe(false)
    expect(result.diagnostics).toEqual([])
    expect(result.overrideSet.overrides).toEqual({})
  })

  it('maps v1 into WEB only and is idempotent', () => {
    const result = migrateLegacyKeybindingOverrideSet(
      legacy([
        {
          commandId: 'spacewave.open',
          disabled: true,
          bindings: [
            {
              id: 'old',
              binding: { case: 'combo', value: { combo: 'x' } },
              surface: CommandSurface.TUI,
            },
          ],
        },
      ]),
      canonical,
    )
    expect(result).toMatchObject({
      required: true,
      diagnostics: [],
      overrideSet: {
        version: 2,
        overrides: { 'spacewave.open': { disabled: true } },
      },
    })
    expect(
      result.overrideSet.overrides['spacewave.open']?.bindings?.[0]?.surface,
    ).toBe(CommandSurface.WEB)
  })

  it('sorts duplicate, empty, and unmapped diagnostics and refuses output', () => {
    const result = migrateLegacyKeybindingOverrideSet(
      legacy([
        { commandId: 'z' },
        { commandId: '' },
        { commandId: 'spacewave.open' },
        { commandId: 'spacewave.open' },
      ]),
      canonical,
    )
    expect(
      result.diagnostics.map((d) => [d.code, d.commandId, d.index]),
    ).toEqual([
      ['empty-command-id', '', 1],
      ['duplicate-command-id', 'spacewave.open', 3],
      ['unmapped-command-id', 'z', 0],
    ])
    expect(result.overrideSet).toEqual({
      version: 2,
      overrides: {},
      settings: { leaderCombo: 'Ctrl+Space' },
    })
  })
})

it('reads and writes the selected TUI partition without inventing WEB state', () => {
  const value: KeybindingOverrideSet = {
    version: 2,
    overrides: {
      'spacewave.open': {
        bindings: [
          {
            id: 'tui',
            binding: { case: 'combo', value: { combo: 'q' } },
            surface: CommandSurface.TUI,
          },
        ],
      },
    },
    settings: { leaderCombo: 'Alt+Space' },
  }
  const proto = keybindingOverrideSetToProto(value, CommandSurface.TUI)
  expect(proto.webOverrides).toEqual([])
  expect(proto.tuiOverrides).toHaveLength(1)
  expect(proto.tuiSettings?.leaderCombo).toBe('Alt+Space')
  const read = keybindingOverrideSetFromProto(proto, CommandSurface.TUI)
  expect(read.overrides['spacewave.open']).toEqual({
    bindings: [
      {
        id: 'tui',
        binding: { case: 'combo', value: { combo: 'q' } },
        surface: CommandSurface.TUI,
      },
    ],
    clearedBindingIds: [],
  })
  expect(read.settings).toEqual(value.settings)
})

it('keeps legacy reads inside the WEB-only migration owner', () => {
  const value = legacy([{ commandId: 'spacewave.open', disabled: true }])
  expect(() =>
    keybindingOverrideSetFromProto(value, CommandSurface.TUI),
  ).toThrow(/requires migration/)
  expect(
    migrateLegacyKeybindingOverrideSet(value, canonical).overrideSet.overrides,
  ).toEqual({
    'spacewave.open': { disabled: true },
  })
})

it('rejects UNKNOWN and cross-surface bindings at serialization boundaries', () => {
  const webValue: KeybindingOverrideSet = {
    version: 2,
    overrides: {
      x: {
        bindings: [
          {
            id: 'x',
            binding: { case: 'combo', value: { combo: 'x' } },
            surface: CommandSurface.TUI,
          },
        ],
      },
    },
    settings: {},
  }
  expect(() =>
    keybindingOverrideSetToProto(webValue, CommandSurface.WEB),
  ).toThrow(/surface/)
  const unknownValue: KeybindingOverrideSet = {
    version: 2,
    overrides: {
      x: {
        bindings: [
          {
            id: 'x',
            binding: { case: 'combo', value: { combo: 'x' } },
            surface: CommandSurface.UNKNOWN,
          },
        ],
      },
    },
    settings: {},
  }
  expect(() =>
    keybindingOverrideSetToProto(unknownValue, CommandSurface.TUI),
  ).toThrow(/surface/)
  expect(() =>
    keybindingOverrideSetToProto(
      webValue,
      CommandSurface.UNKNOWN as CommandSurface,
    ),
  ).toThrow(/WEB or TUI/)
})

it('preserves the non-selected partition when merging account or Space writes', () => {
  const current: ProtoKeybindingOverrideSet = {
    version: 2,
    overrides: [],
    webOverrides: [
      {
        commandId: 'web',
        bindings: [
          {
            id: 'web',
            binding: { case: 'combo', value: { combo: 'w' } },
            surface: CommandSurface.WEB,
          },
        ],
      },
    ],
    tuiOverrides: [
      {
        commandId: 'tui',
        bindings: [
          {
            id: 'tui',
            binding: { case: 'combo', value: { combo: 't' } },
            surface: CommandSurface.TUI,
          },
        ],
      },
    ],
  }
  const next: KeybindingOverrideSet = {
    version: 2,
    overrides: {
      next: {
        bindings: [
          {
            id: 'next',
            binding: { case: 'combo', value: { combo: 'n' } },
            surface: CommandSurface.TUI,
          },
        ],
      },
    },
    settings: {},
  }
  const merged = mergeKeybindingOverridePartitions(
    current,
    next,
    CommandSurface.TUI,
  )
  expect(merged.webOverrides?.map((x) => x.commandId)).toEqual(['web'])
  expect(merged.tuiOverrides?.map((x) => x.commandId)).toEqual(['next'])
})
