import { expect, it } from 'vitest'
import {
  CommandSurface,
  type KeybindingOverrideSet as ProtoKeybindingOverrideSet,
} from '@s4wave/sdk/command/command.pb.js'
import {
  keybindingOverrideSetFromProto,
  keybindingOverrideSetToProto,
  mergeKeybindingOverridePartitions,
  type KeybindingOverrideSet,
} from './keybinding-overrides.js'

it('reads and writes the selected TUI partition without inventing WEB state', () => {
  const value: KeybindingOverrideSet = {
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

it('rejects UNKNOWN and cross-surface bindings at serialization boundaries', () => {
  const webValue: KeybindingOverrideSet = {
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
