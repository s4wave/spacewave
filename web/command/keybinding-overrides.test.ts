import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import { describe, expect, it } from 'vitest'
import {
  CommandFocusContext,
  KeybindingDisplayMode,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  clearCommandBindingsOverride,
  clearKeybindingOverrideSet,
  createEmptyKeybindingOverrideSet,
  keybindingBindingStorageId,
  keybindingOverrideSetFromProto,
  keybindingOverrideSetToProto,
  normalizeKeybindingOverrideSet,
  resetKeybindingCommandOverride,
  type KeybindingOverrideSet as ModelKeybindingOverrideSet,
  setCommandBindingsOverride,
  setKeybindingCommandOverride,
} from './keybinding-overrides.js'

function comboBinding(
  id: string,
  combo: string,
  when = CommandFocusContext.GLOBAL,
): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when,
    surface: CommandSurface.WEB,
  }
}

function sequenceBinding(id: string, steps: string[]): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when: CommandFocusContext.GLOBAL,
    surface: CommandSurface.WEB,
  }
}

describe('keybinding local override schema', () => {
  it('normalizes persisted command-id keyed overrides and drops malformed entries', () => {
    const normalized = normalizeKeybindingOverrideSet({
      version: 99,
      overrides: {
        '': { disabled: true },
        'spacewave.palette': {
          replaceBindings: true,
          disabled: 'yes',
          clearedBindingIds: ['default-palette', '', 'default-palette', 0],
          bindings: [
            comboBinding('local-palette', 'Ctrl+K'),
            { id: 'bad-no-binding' },
            { binding: { case: 'unset', value: {} } },
          ],
          label: 'Command Palette',
          menuPath: 'View/Command Palette',
        },
        'spacewave.empty': {
          replaceBindings: false,
          clearedBindingIds: [],
          bindings: [{ id: 'bad' }],
        },
        'spacewave.malformed': 'disabled',
      },
      settings: {
        leaderCombo: 'Alt+Space',
        whichKeyDelayMs: 125,
        display: { mode: 'symbols' },
        ignored: true,
      },
    })

    expect(normalized).toEqual({
      overrides: {
        'spacewave.palette': {
          replaceBindings: true,
          clearedBindingIds: ['default-palette'],
          bindings: [comboBinding('local-palette', 'Ctrl+K')],
        },
      },
      settings: {
        leaderCombo: 'Alt+Space',
        whichKeyDelayMs: 125,
        display: { mode: 'symbols' },
      },
    })
    expect(
      Object.keys(normalized.overrides['spacewave.palette']).sort(),
    ).toEqual(['bindings', 'clearedBindingIds', 'replaceBindings'])
  })

  it('adds, replaces, clears, disables, and resets command overrides without command metadata', () => {
    const empty = createEmptyKeybindingOverrideSet()
    const withAdded = addCommandBindingOverride(
      empty,
      'spacewave.palette',
      comboBinding('local-palette', 'Ctrl+K'),
    )

    expect(withAdded.overrides['spacewave.palette']).toEqual({
      bindings: [comboBinding('local-palette', 'Ctrl+K')],
    })

    const withReplacement = setCommandBindingsOverride(
      withAdded,
      'spacewave.palette',
      [sequenceBinding('local-palette-sequence', ['Leader', 'P'])],
    )

    expect(withReplacement.overrides['spacewave.palette']).toEqual({
      replaceBindings: true,
      bindings: [sequenceBinding('local-palette-sequence', ['Leader', 'P'])],
    })

    const withOneDefaultCleared = clearCommandBindingIdOverride(
      withReplacement,
      'spacewave.palette',
      'default-palette',
    )

    expect(withOneDefaultCleared.overrides['spacewave.palette']).toEqual({
      replaceBindings: true,
      clearedBindingIds: ['default-palette'],
      bindings: [sequenceBinding('local-palette-sequence', ['Leader', 'P'])],
    })

    const cleared = clearCommandBindingsOverride(
      withOneDefaultCleared,
      'spacewave.palette',
    )

    expect(cleared.overrides['spacewave.palette']).toEqual({
      replaceBindings: true,
      bindings: [],
    })

    const disabled = setKeybindingCommandOverride(
      cleared,
      'spacewave.palette',
      { disabled: true },
    )

    expect(disabled.overrides['spacewave.palette']).toEqual({ disabled: true })
    expect(Object.keys(disabled.overrides['spacewave.palette']).sort()).toEqual(
      ['disabled'],
    )

    const withSecondCommand = addCommandBindingOverride(
      disabled,
      'spacewave.quick-open',
      comboBinding('local-quick-open', 'Ctrl+O'),
    )
    const resetOne = resetKeybindingCommandOverride(
      withSecondCommand,
      'spacewave.palette',
    )

    expect(resetOne.overrides).toEqual({
      'spacewave.quick-open': {
        bindings: [comboBinding('local-quick-open', 'Ctrl+O')],
      },
    })
    expect(clearKeybindingOverrideSet()).toEqual({
      overrides: {},
      settings: {},
    })
  })

  it('derives stable local binding ids for bindings that do not persist ids', () => {
    expect(
      keybindingBindingStorageId({
        binding: { case: 'combo', value: { combo: 'Ctrl+K' } },
      }),
    ).toBe('combo:Ctrl+K')
    expect(
      keybindingBindingStorageId({
        binding: { case: 'sequence', value: { steps: ['Leader', 'K'] } },
      }),
    ).toBe('sequence:Leader K')
  })

  it('round-trips the shared proto override set without losing command keys, binding ids, disables, clears, or display settings', () => {
    const model: ModelKeybindingOverrideSet = {
      overrides: {
        'spacewave.palette': {
          replaceBindings: true,
          clearedBindingIds: ['palette-default', 'palette-alt'],
          bindings: [
            comboBinding('palette-replace', 'Ctrl+K'),
            sequenceBinding('palette-leader', ['Leader', 'P']),
          ],
        },
        'spacewave.disabled': {
          disabled: true,
          bindings: [],
        },
        'spacewave.cleared': {
          clearedBindingIds: ['cleared-default'],
          bindings: [],
        },
      },
      settings: {
        leaderCombo: 'Ctrl+Alt+Space',
        whichKeyDelayMs: 275,
        display: { mode: 'text' as const },
      },
    }

    const proto = keybindingOverrideSetToProto(model)
    const sortedOverrides = [...(proto.webOverrides ?? [])].sort((a, b) =>
      (a.commandId ?? '').localeCompare(b.commandId ?? ''),
    )

    expect(proto.webSettings).toEqual({
      leaderCombo: 'Ctrl+Alt+Space',
      whichKeyDelayMs: 275,
      display: { mode: KeybindingDisplayMode.TEXT },
    })
    expect(sortedOverrides).toEqual([
      expect.objectContaining({
        commandId: 'spacewave.cleared',
        clearedBindingIds: ['cleared-default'],
        bindings: [],
      }),
      expect.objectContaining({
        commandId: 'spacewave.disabled',
        disabled: true,
        bindings: [],
      }),
      expect.objectContaining({
        commandId: 'spacewave.palette',
        replaceBindings: true,
        clearedBindingIds: ['palette-default', 'palette-alt'],
        bindings: [
          comboBinding('palette-replace', 'Ctrl+K'),
          sequenceBinding('palette-leader', ['Leader', 'P']),
        ],
      }),
    ])
    const roundTripped = keybindingOverrideSetFromProto(proto)
    expect(roundTripped.settings).toEqual(model.settings)
    expect(roundTripped.overrides['spacewave.cleared']).toEqual(
      model.overrides['spacewave.cleared'],
    )
    expect(roundTripped.overrides['spacewave.disabled']).toEqual(
      expect.objectContaining({
        disabled: true,
        bindings: [],
      }),
    )
    expect(roundTripped.overrides['spacewave.palette']).toEqual(
      model.overrides['spacewave.palette'],
    )
  })
})
