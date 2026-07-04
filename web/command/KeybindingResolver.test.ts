import { describe, expect, it } from 'vitest'
import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  contextKey,
  getCommandDisplayBindings,
  normalizeKeyCombo,
  resolveKeybindings,
  selectActiveComboMatch,
  selectActiveSequenceNode,
} from './KeybindingResolver.js'
import type { KeybindingOverrideLayer } from './keybinding-overrides.js'

function commandState(
  command: Command,
  state: Pick<CommandState, 'active' | 'enabled'> = {},
): CommandState {
  return {
    active: state.active ?? true,
    enabled: state.enabled ?? true,
    command,
  }
}

function command(
  commandId: string,
  overrides: Omit<Command, 'commandId' | 'label'> = {},
): Command {
  return {
    commandId,
    label: commandId,
    ...overrides,
  }
}

function comboBinding(
  id: string,
  combo: string,
  when = CommandFocusContext.GLOBAL,
): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when,
  }
}

function sequenceBinding(
  id: string,
  steps: string[],
  when = CommandFocusContext.GLOBAL,
): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when,
  }
}

describe('KeybindingResolver', () => {
  it('migrates a legacy keybinding to one global default combo when typed defaults are empty', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.legacy.open', {
            keybinding: 'CmdOrCtrl+K',
            defaultBindings: [],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    const bindings = graph.bindingsByCommandId.get('spacewave.legacy.open')

    expect(
      bindings?.map((binding) => ({
        bindingId: binding.bindingId,
        combo: binding.combo,
        context: binding.context,
        kind: binding.kind,
      })),
    ).toEqual([
      {
        bindingId: 'legacy-keybinding',
        combo: 'meta+k',
        context: CommandFocusContext.GLOBAL,
        kind: 'combo',
      },
    ])
    expect(
      graph.comboBindings.get(contextKey(CommandFocusContext.GLOBAL, 'meta+k')),
    ).toBe(bindings?.[0])
    expect(bindings?.[0]?.source).toEqual({
      id: 'legacy-keybinding',
      binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
      when: CommandFocusContext.GLOBAL,
    })
    expect(graph.conflicts).toEqual([])
  })

  it('uses typed default bindings instead of the legacy keybinding when both are present', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.precedence.open', {
            keybinding: 'CmdOrCtrl+K',
            defaultBindings: [
              comboBinding('typed-open', 'Alt+O', CommandFocusContext.EDITOR),
            ],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    const bindings = graph.bindingsByCommandId.get('spacewave.precedence.open')

    expect(
      bindings?.map((binding) => ({
        bindingId: binding.bindingId,
        combo: binding.combo,
        context: binding.context,
        display: binding.display,
      })),
    ).toEqual([
      {
        bindingId: 'typed-open',
        combo: 'alt+o',
        context: CommandFocusContext.EDITOR,
        display: 'Alt+O',
      },
    ])
    expect(
      graph.comboBindings.has(contextKey(CommandFocusContext.GLOBAL, 'meta+k')),
    ).toBe(false)
    expect(
      graph.comboBindings.get(contextKey(CommandFocusContext.EDITOR, 'alt+o')),
    ).toBe(bindings?.[0])
  })

  it('expands CmdOrCtrl according to the resolver platform option', () => {
    const commands = [
      commandState(
        command('spacewave.palette.toggle', {
          defaultBindings: [comboBinding('palette', 'CmdOrCtrl+Shift+P')],
        }),
      ),
    ]

    const macGraph = resolveKeybindings(commands, { platform: 'mac' })
    const otherGraph = resolveKeybindings(commands, { platform: 'other' })

    expect(
      macGraph.comboBindings.get(
        contextKey(CommandFocusContext.GLOBAL, 'meta+shift+p'),
      )?.commandId,
    ).toBe('spacewave.palette.toggle')
    expect(
      otherGraph.comboBindings.get(
        contextKey(CommandFocusContext.GLOBAL, 'ctrl+shift+p'),
      )?.commandId,
    ).toBe('spacewave.palette.toggle')
    expect(normalizeKeyCombo('CmdOrCtrl+Shift+P', 'mac')).toBe('meta+shift+p')
    expect(normalizeKeyCombo('CmdOrCtrl+Shift+P', 'other')).toBe('ctrl+shift+p')
  })

  it('filters inactive and disabled commands out of the resolved graph', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.active', {
            defaultBindings: [comboBinding('active', 'Ctrl+A')],
          }),
        ),
        commandState(
          command('spacewave.inactive', {
            defaultBindings: [comboBinding('inactive', 'Ctrl+I')],
          }),
          { active: false },
        ),
        commandState(
          command('spacewave.disabled', {
            defaultBindings: [comboBinding('disabled', 'Ctrl+D')],
          }),
          { enabled: false },
        ),
      ],
      { platform: 'other' },
    )

    expect([...graph.bindingsByCommandId.keys()]).toEqual(['spacewave.active'])
    expect([...graph.comboBindings.keys()]).toEqual([
      contextKey(CommandFocusContext.GLOBAL, 'ctrl+a'),
    ])
    expect(graph.conflicts).toEqual([])
  })

  it('records duplicate same-context combos as conflicts without overwriting combo dispatch', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.first', {
            defaultBindings: [
              comboBinding(
                'first-editor',
                'Ctrl+K',
                CommandFocusContext.EDITOR,
              ),
            ],
          }),
        ),
        commandState(
          command('spacewave.second', {
            defaultBindings: [
              comboBinding(
                'second-editor',
                'Ctrl+K',
                CommandFocusContext.EDITOR,
              ),
            ],
          }),
        ),
      ],
      { platform: 'other' },
    )

    const key = contextKey(CommandFocusContext.EDITOR, 'ctrl+k')
    const conflict = graph.comboConflicts.get(key)

    expect(graph.comboBindings.has(key)).toBe(false)
    expect(conflict).toBeDefined()
    expect(conflict?.context).toBe(CommandFocusContext.EDITOR)
    expect(conflict?.kind).toBe('combo')
    expect(conflict?.key).toBe('ctrl+k')
    expect(conflict?.bindings.map((binding) => binding.commandId)).toEqual([
      'spacewave.first',
      'spacewave.second',
    ])
    expect(graph.conflicts).toEqual([conflict])
  })

  it('builds sequence bindings into a trie and expands Leader to Ctrl+Space by default', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.sequence.open', {
            defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    const leaderNode = graph.sequenceTrie.children.get('ctrl+space')
    const openNode = leaderNode?.children.get('o')
    const binding = openNode?.bindings[0]

    expect(leaderNode?.step).toBe('ctrl+space')
    expect(openNode?.step).toBe('o')
    expect(binding?.commandId).toBe('spacewave.sequence.open')
    expect(binding?.bindingId).toBe('leader-open')
    expect(binding?.context).toBe(CommandFocusContext.GLOBAL)
    expect(binding?.kind).toBe('sequence')
    expect(binding?.sequence).toEqual(['ctrl+space', 'o'])
    expect(binding?.display).toBe('Leader O')
    expect(openNode?.conflicts).toEqual([])
  })

  it('resolves bindings for every typed command focus context', () => {
    const contexts = [
      CommandFocusContext.GLOBAL,
      CommandFocusContext.SHELL_TAB,
      CommandFocusContext.EDITOR,
      CommandFocusContext.LIST,
      CommandFocusContext.CANVAS,
      CommandFocusContext.MODAL,
      CommandFocusContext.TEXT_INPUT,
    ]

    const graph = resolveKeybindings(
      contexts.map((context) =>
        commandState(
          command(`spacewave.context.${context}`, {
            defaultBindings: [
              comboBinding(`binding-${context}`, 'Alt+K', context),
            ],
          }),
        ),
      ),
      { platform: 'other' },
    )

    for (const context of contexts) {
      expect(
        graph.comboBindings.get(contextKey(context, 'alt+k'))?.commandId,
      ).toBe(`spacewave.context.${context}`)
    }
  })

  it('selects the most specific active focus context before conflict checks', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.global.first', {
            defaultBindings: [comboBinding('global-first', 'CmdOrCtrl+K')],
          }),
        ),
        commandState(
          command('spacewave.global.second', {
            defaultBindings: [comboBinding('global-second', 'CmdOrCtrl+K')],
          }),
        ),
        commandState(
          command('notes.insert.link', {
            defaultBindings: [
              comboBinding(
                'editor-link',
                'CmdOrCtrl+K',
                CommandFocusContext.EDITOR,
              ),
            ],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    const editorMatch = selectActiveComboMatch(
      graph,
      [
        CommandFocusContext.GLOBAL,
        CommandFocusContext.SHELL_TAB,
        CommandFocusContext.EDITOR,
      ],
      'meta+k',
    )
    const globalMatch = selectActiveComboMatch(
      graph,
      [CommandFocusContext.GLOBAL],
      'meta+k',
    )

    expect(
      graph.comboConflicts.get(
        contextKey(CommandFocusContext.GLOBAL, 'meta+k'),
      ),
    ).toBeDefined()
    expect(editorMatch?.binding?.commandId).toBe('notes.insert.link')
    expect(editorMatch?.conflict).toBeUndefined()
    expect(globalMatch?.binding).toBeUndefined()
    expect(
      globalMatch?.conflict?.bindings.map((binding) => binding.commandId),
    ).toEqual(['spacewave.global.first', 'spacewave.global.second'])
  })

  it('keeps global bindings inert in text inputs without a text-input binding', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.palette', {
            defaultBindings: [comboBinding('global-palette', 'CmdOrCtrl+K')],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    const match = selectActiveComboMatch(
      graph,
      [CommandFocusContext.GLOBAL, CommandFocusContext.TEXT_INPUT],
      'meta+k',
    )

    expect(
      graph.comboBindings.get(contextKey(CommandFocusContext.GLOBAL, 'meta+k'))
        ?.commandId,
    ).toBe('spacewave.palette')
    expect(match).toBeUndefined()
  })

  it('filters sequence prefixes with active focus-context rules at lookup time', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.global.open', {
            defaultBindings: [sequenceBinding('global-open', ['Leader', 'O'])],
          }),
        ),
        commandState(
          command('notes.insert.link', {
            defaultBindings: [
              sequenceBinding(
                'editor-link',
                ['Leader', 'K'],
                CommandFocusContext.EDITOR,
              ),
            ],
          }),
        ),
      ],
      { platform: 'other' },
    )

    const leaderNode = graph.sequenceTrie.children.get('ctrl+space')
    expect(leaderNode?.children.has('o')).toBe(true)
    expect(leaderNode?.children.has('k')).toBe(true)

    const editorLeaderNode = leaderNode
      ? selectActiveSequenceNode(leaderNode, [
          CommandFocusContext.GLOBAL,
          CommandFocusContext.EDITOR,
        ])
      : undefined

    expect(editorLeaderNode?.children.has('o')).toBe(false)
    expect(editorLeaderNode?.children.get('k')?.bindings[0]?.commandId).toBe(
      'notes.insert.link',
    )
  })

  it('shows context labels only when same binding text appears in multiple contexts', () => {
    const graph = resolveKeybindings(
      [
        commandState(
          command('spacewave.palette', {
            defaultBindings: [
              comboBinding('global-palette', 'CmdOrCtrl+K'),
              comboBinding('global-help', 'Ctrl+/', CommandFocusContext.GLOBAL),
            ],
          }),
        ),
        commandState(
          command('notes.insert.link', {
            defaultBindings: [
              comboBinding(
                'editor-link',
                'CmdOrCtrl+K',
                CommandFocusContext.EDITOR,
              ),
            ],
          }),
        ),
      ],
      { platform: 'mac' },
    )

    expect(getCommandDisplayBindings(graph, 'spacewave.palette')).toEqual([
      'CmdOrCtrl+K (Global)',
      'Ctrl+/',
    ])
    expect(getCommandDisplayBindings(graph, 'notes.insert.link')).toEqual([
      'CmdOrCtrl+K (Editor)',
    ])
  })

  it('applies a local override layer above defaults for clearing, replacement, addition, disabling, and reset fallback', () => {
    const commands = [
      commandState(
        command('spacewave.palette', {
          defaultBindings: [
            comboBinding('palette-default', 'Ctrl+P'),
            comboBinding('palette-alt', 'Ctrl+Shift+P'),
          ],
        }),
      ),
      commandState(
        command('spacewave.quick-open', {
          defaultBindings: [comboBinding('quick-open-default', 'Ctrl+O')],
        }),
      ),
      commandState(
        command('spacewave.disabled', {
          defaultBindings: [comboBinding('disabled-default', 'Ctrl+D')],
        }),
      ),
      commandState(
        command('spacewave.reset', {
          defaultBindings: [comboBinding('reset-default', 'Ctrl+R')],
        }),
      ),
    ]
    const localLayer: KeybindingOverrideLayer = {
      scope: 'local',
      label: 'Local',
      overrideSet: {
        version: 1,
        overrides: {
          'spacewave.palette': {
            clearedBindingIds: ['palette-alt'],
            bindings: [comboBinding('palette-local', 'Ctrl+K')],
          },
          'spacewave.quick-open': {
            replaceBindings: true,
            bindings: [sequenceBinding('quick-open-local', ['Leader', 'O'])],
          },
          'spacewave.disabled': {
            disabled: true,
          },
        },
        settings: {},
      },
    }

    const graph = resolveKeybindings(commands, {
      platform: 'other',
      overrideLayers: [localLayer],
    })

    expect(
      graph.bindingsByCommandId.get('spacewave.palette')?.map((binding) => ({
        id: binding.bindingId,
        display: binding.display,
        layer: binding.sourceLayer,
        label: binding.sourceLayerLabel,
      })),
    ).toEqual([
      {
        id: 'palette-default',
        display: 'Ctrl+P',
        layer: 'default',
        label: 'Default',
      },
      {
        id: 'palette-local',
        display: 'Ctrl+K',
        layer: 'local',
        label: 'Local',
      },
    ])
    expect(
      graph.comboBindings.has(
        contextKey(CommandFocusContext.GLOBAL, 'ctrl+shift+p'),
      ),
    ).toBe(false)
    expect(
      graph.bindingsByCommandId.get('spacewave.quick-open')?.map((binding) => ({
        id: binding.bindingId,
        sequence: binding.sequence,
        layer: binding.sourceLayer,
      })),
    ).toEqual([
      {
        id: 'quick-open-local',
        sequence: ['ctrl+space', 'o'],
        layer: 'local',
      },
    ])
    expect(graph.bindingsByCommandId.has('spacewave.disabled')).toBe(false)
    expect(
      graph.bindingsByCommandId.get('spacewave.reset')?.map((binding) => ({
        id: binding.bindingId,
        layer: binding.sourceLayer,
      })),
    ).toEqual([{ id: 'reset-default', layer: 'default' }])
  })

  it('does not apply account or space layers unless passed and keeps the cascade ready for space over account over local over default', () => {
    const commands = [
      commandState(
        command('spacewave.palette', {
          defaultBindings: [comboBinding('palette-default', 'Ctrl+P')],
        }),
      ),
    ]
    const localLayer: KeybindingOverrideLayer = {
      scope: 'local',
      label: 'Local',
      overrideSet: {
        version: 1,
        overrides: {
          'spacewave.palette': {
            replaceBindings: true,
            bindings: [comboBinding('palette-local', 'Ctrl+L')],
          },
        },
        settings: {},
      },
    }
    const accountLayer: KeybindingOverrideLayer = {
      scope: 'account',
      label: 'Account',
      overrideSet: {
        version: 1,
        overrides: {
          'spacewave.palette': {
            replaceBindings: true,
            bindings: [comboBinding('palette-account', 'Ctrl+A')],
          },
        },
        settings: {},
      },
    }
    const spaceLayer: KeybindingOverrideLayer = {
      scope: 'space',
      label: 'Space',
      overrideSet: {
        version: 1,
        overrides: {
          'spacewave.palette': {
            replaceBindings: true,
            bindings: [comboBinding('palette-space', 'Ctrl+S')],
          },
        },
        settings: {},
      },
    }

    expect(
      getCommandDisplayBindings(
        resolveKeybindings(commands, { platform: 'other' }),
        'spacewave.palette',
      ),
    ).toEqual(['Ctrl+P'])
    expect(
      getCommandDisplayBindings(
        resolveKeybindings(commands, {
          platform: 'other',
          overrideLayers: [localLayer],
        }),
        'spacewave.palette',
      ),
    ).toEqual(['Ctrl+L'])

    const graph = resolveKeybindings(commands, {
      platform: 'other',
      overrideLayers: [localLayer, accountLayer, spaceLayer],
    })
    const binding = graph.bindingsByCommandId.get('spacewave.palette')?.[0]

    expect(binding?.display).toBe('Ctrl+S')
    expect(binding?.sourceLayer).toBe('space')
    expect(binding?.sourceLayerLabel).toBe('Space')
    expect(
      graph.comboBindings.has(contextKey(CommandFocusContext.GLOBAL, 'ctrl+l')),
    ).toBe(false)
    expect(
      graph.comboBindings.has(contextKey(CommandFocusContext.GLOBAL, 'ctrl+a')),
    ).toBe(false)
  })

  it('records local same-context combo conflicts without dispatching either conflicting binding', () => {
    const localLayer: KeybindingOverrideLayer = {
      scope: 'local',
      label: 'Local',
      overrideSet: {
        version: 1,
        overrides: {
          'spacewave.first': {
            bindings: [
              comboBinding('first-local', 'Ctrl+K', CommandFocusContext.EDITOR),
            ],
          },
          'spacewave.second': {
            bindings: [
              comboBinding(
                'second-local',
                'Ctrl+K',
                CommandFocusContext.EDITOR,
              ),
            ],
          },
        },
        settings: {},
      },
    }
    const graph = resolveKeybindings(
      [
        commandState(command('spacewave.first')),
        commandState(command('spacewave.second')),
      ],
      { platform: 'other', overrideLayers: [localLayer] },
    )
    const key = contextKey(CommandFocusContext.EDITOR, 'ctrl+k')
    const conflict = graph.comboConflicts.get(key)

    expect(graph.comboBindings.has(key)).toBe(false)
    expect(
      conflict?.bindings.map((binding) => ({
        commandId: binding.commandId,
        layer: binding.sourceLayer,
        label: binding.sourceLayerLabel,
      })),
    ).toEqual([
      { commandId: 'spacewave.first', layer: 'local', label: 'Local' },
      { commandId: 'spacewave.second', layer: 'local', label: 'Local' },
    ])
    expect(graph.conflicts).toEqual([conflict])
  })
})
