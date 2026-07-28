import { Window } from 'happy-dom'
import type { ReactNode, Ref } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react'
import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  CommandPalette,
  formatKeybinding,
  formatKeybindingHint,
} from './CommandPalette.js'
import { createSubItemQueryId } from './sub-item-navigation.js'

if (typeof document === 'undefined') {
  const happyDomWindow = new Window({ url: 'http://localhost/' })

  Object.defineProperties(globalThis, {
    window: { value: happyDomWindow, configurable: true },
    document: { value: happyDomWindow.document, configurable: true },
    HTMLElement: { value: happyDomWindow.HTMLElement, configurable: true },
    Element: { value: happyDomWindow.Element, configurable: true },
    Node: { value: happyDomWindow.Node, configurable: true },
    Text: { value: happyDomWindow.Text, configurable: true },
    DocumentFragment: {
      value: happyDomWindow.DocumentFragment,
      configurable: true,
    },
    SVGElement: { value: happyDomWindow.SVGElement, configurable: true },
    Event: { value: happyDomWindow.Event, configurable: true },
    CustomEvent: { value: happyDomWindow.CustomEvent, configurable: true },
    KeyboardEvent: { value: happyDomWindow.KeyboardEvent, configurable: true },
    MouseEvent: { value: happyDomWindow.MouseEvent, configurable: true },
    FocusEvent: { value: happyDomWindow.FocusEvent, configurable: true },
    InputEvent: { value: happyDomWindow.InputEvent, configurable: true },
    MutationObserver: {
      value: happyDomWindow.MutationObserver,
      configurable: true,
    },
  })
}

let mockCommands: CommandState[] = []
let registeredPaletteCommand:
  | {
      commandId: string
      defaultBindings?: CommandBinding[]
    }
  | undefined
let paletteHandler: (() => void) | undefined
const mockInvokeCommand = vi.fn()
const mockGetSubItems = vi.fn()
const mockRegisterOpenCommand = vi.fn(() => () => {})

vi.mock('./CommandContext.js', () => ({
  useCommands: () => mockCommands,
  useInvokeCommand: () => mockInvokeCommand,
  useCommandContext: () => ({
    getSubItems: mockGetSubItems,
    registerOpenCommand: mockRegisterOpenCommand,
  }),
}))

vi.mock('./useCommand.js', () => ({
  useCommand: (opts: {
    commandId: string
    defaultBindings?: CommandBinding[]
    handler: () => void
  }) => {
    if (opts.commandId === 'spacewave.view.palette') {
      registeredPaletteCommand = opts
      paletteHandler = opts.handler
    }
  },
}))

vi.mock('@s4wave/web/ui/command.js', () => ({
  CommandDialog: ({
    open,
    children,
  }: {
    open: boolean
    children: ReactNode
  }) => (open ? <div role="dialog">{children}</div> : null),
  CommandEmpty: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  CommandFooter: () => <div />,
  CommandGroup: ({
    heading,
    children,
  }: {
    heading: string
    children: ReactNode
  }) => (
    <section>
      <h3>{heading}</h3>
      {children}
    </section>
  ),
  CommandInput: ({
    onClick,
    onValueChange,
    placeholder,
    ref,
    value,
  }: {
    onClick?: () => void
    onValueChange?: (value: string) => void
    placeholder?: string
    ref?: Ref<HTMLInputElement>
    value?: string
  }) => (
    <input
      aria-label="Command search"
      onChange={(event) => onValueChange?.(event.currentTarget.value)}
      onInput={(event) => onValueChange?.(event.currentTarget.value)}
      onClick={onClick}
      placeholder={placeholder}
      ref={ref}
      value={value ?? ''}
    />
  ),
  CommandItem: ({
    children,
    disabled,
    onSelect,
  }: {
    children: ReactNode
    disabled?: boolean
    onSelect?: () => void
    value?: string
  }) => (
    <button disabled={disabled} onClick={onSelect} type="button">
      {children}
    </button>
  ),
  CommandList: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandShortcut: ({ children }: { children: ReactNode }) => (
    <span>{children}</span>
  ),
}))

function textContentMatches(...texts: string[]) {
  return (_content: string, node: Element | null) => {
    if (!node || !texts.includes(node.textContent?.trim() ?? '')) return false
    return Array.from(node.children).every(
      (child) => !texts.includes(child.textContent?.trim() ?? ''),
    )
  }
}

describe('CommandPalette', () => {
  afterEach(() => {
    cleanup()
    mockCommands = []
    paletteHandler = undefined
    registeredPaletteCommand = undefined
    vi.clearAllMocks()
  })

  it('registers the palette command with combo and leader-sequence bindings', () => {
    render(<CommandPalette />)

    expect(registeredPaletteCommand?.commandId).toBe('spacewave.view.palette')
    expect(registeredPaletteCommand?.defaultBindings).toEqual([
      {
        id: 'global-palette',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
        when: CommandFocusContext.GLOBAL,
      },
      {
        id: 'global-palette-sequence',
        binding: { case: 'sequence', value: { steps: ['Leader', 'Space'] } },
        when: CommandFocusContext.GLOBAL,
      },
    ])
  })

  it('shows legacy keybinding display through the resolver migration path', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.view.help',
          label: 'Open Help',
          keybinding: 'Ctrl+H',
          menuPath: 'Help/Open Help',
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(view.getByRole('dialog')).toBeTruthy()
    expect(view.getByText('Open Help')).toBeTruthy()
    expect(view.getByText(textContentMatches('⌃H', 'Ctrl+H'))).toBeTruthy()
  })

  it('renders one modifier-bearing hint for duplicate bindings', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.nav.back',
          label: 'Navigate Back',
          menuPath: 'File/Navigate Back',
          defaultBindings: [
            {
              id: 'back-default',
              binding: { case: 'combo', value: { combo: 'Alt+ArrowLeft' } },
            },
            {
              id: 'back-browser-alias',
              binding: { case: 'combo', value: { combo: 'Alt+ArrowLeft' } },
            },
            {
              id: 'back-platform-alias',
              binding: { case: 'combo', value: { combo: 'Alt+ArrowLeft' } },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(
      view.getByText(textContentMatches('Alt+ArrowLeft', '\u2325ArrowLeft')),
    ).toBeTruthy()
    expect(
      view.queryByText(
        textContentMatches(
          'Alt+ArrowLeft / Alt+ArrowLeft / Alt+ArrowLeft',
          '\u2325ArrowLeft / \u2325ArrowLeft / \u2325ArrowLeft',
        ),
      ),
    ).toBeNull()
  })

  it('formats Option as the platform Alt modifier', () => {
    const formatted = formatKeybinding('Option+ArrowLeft')

    expect(formatted).toMatch(/^(Alt\+ArrowLeft|\u2325ArrowLeft)$/)
    expect(
      formatKeybindingHint(['Alt+ArrowLeft', 'Alt+ArrowLeft', 'Alt+ArrowLeft']),
    ).toMatch(/^(Alt\+ArrowLeft|\u2325ArrowLeft)$/)
  })

  it('shows plural typed default bindings on command rows', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-combo',
              binding: { case: 'combo', value: { combo: 'Ctrl+O' } },
            },
            {
              id: 'open-sequence',
              binding: {
                case: 'sequence',
                value: { steps: ['Leader', 'F', 'O'] },
              },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(view.getByText('Open File')).toBeTruthy()
    expect(
      view.getByText(
        textContentMatches('⌃O / Leader F O', 'Ctrl+O / Leader F O'),
      ),
    ).toBeTruthy()
  })

  it('shows context labels when same binding text appears in multiple contexts', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.view.palette',
          label: 'Command Palette',
          menuPath: 'View/Command Palette',
          defaultBindings: [
            {
              id: 'palette',
              binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
      {
        command: {
          commandId: 'notes.insert.link',
          label: 'Insert Link',
          menuPath: 'Edit/Insert Link',
          defaultBindings: [
            {
              id: 'insert-link',
              binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
              when: CommandFocusContext.EDITOR,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(view.getByText('Command Palette')).toBeTruthy()
    expect(
      view.getByText(
        textContentMatches(
          '⌘K (Global)',
          'CmdOrCtrl+K (Global)',
          'Ctrl+K (Global)',
        ),
      ),
    ).toBeTruthy()
    expect(view.getByText('Insert Link')).toBeTruthy()
    expect(
      view.getByText(
        textContentMatches(
          '⌘K (Editor)',
          'CmdOrCtrl+K (Editor)',
          'Ctrl+K (Editor)',
        ),
      ),
    ).toBeTruthy()
  })

  it('finds the keyboard shortcut editor by keybind search text', async () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.preferences.keyboard-shortcuts',
          label: 'Edit Keyboard Shortcuts',
          description: 'Customize local keybindings',
          menuPath: 'Tools/Keyboard Shortcuts',
        },
        active: true,
        enabled: true,
      },
      {
        command: {
          commandId: 'spacewave.view.banana',
          label: 'Launch Banana',
          description: 'Open the unrelated banana surface',
          menuPath: 'Tools/Banana',
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    act(() => {
      fireEvent.input(view.getByLabelText('Command search'), {
        target: { value: 'keybind' },
      })
    })

    await waitFor(() => {
      expect(
        view.getByText(textContentMatches('Edit Keyboard Shortcuts')),
      ).toBeTruthy()
      const label = view.getByText(
        textContentMatches('Edit Keyboard Shortcuts'),
      )
      expect(
        Array.from(label.querySelectorAll('span')).some(
          (span) =>
            span.className.includes('text-brand') &&
            /Keyboard|Shortcuts/.test(span.textContent ?? ''),
        ),
      ).toBe(true)
      expect(view.queryByText('Launch Banana')).toBeNull()
    })
  })

  it('finds commands only through declared search aliases', async () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.object.open',
          label: 'Browse Objects',
          description: 'Choose an object in the active space',
          menuPath: 'Go/Browse Objects',
          searchAliases: ['select'],
        },
        active: true,
        enabled: true,
      },
      {
        command: {
          commandId: 'spacewave.view.banana',
          label: 'Launch Banana',
          description: 'Open the unrelated banana surface',
          menuPath: 'Tools/Banana',
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    act(() => {
      fireEvent.input(view.getByLabelText('Command search'), {
        target: { value: 'select' },
      })
    })

    await waitFor(() => {
      expect(view.getByText(textContentMatches('Browse Objects'))).toBeTruthy()
      expect(view.queryByText('Launch Banana')).toBeNull()
    })
  })

  it('renders sub-item icons, descriptions, and experimental badges', async () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.create-object',
          label: 'Create Object',
          menuPath: 'File/Create Object',
          hasSubItems: true,
        },
        active: true,
        enabled: true,
      },
    ]
    mockGetSubItems.mockResolvedValue([
      {
        id: 'canvas',
        label: 'Canvas',
        description: 'Freeform canvas for drawing and layout',
        iconName: 'LuLayoutGrid',
      },
      {
        id: 'notes/blog',
        label: 'Blog',
        description: 'Content',
        experimental: true,
      },
    ])

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    fireEvent.click(view.getByText('Create Object'))

    await waitFor(() => {
      expect(view.getByText('Canvas')).toBeTruthy()
    })
    expect(
      view.getByText('Freeform canvas for drawing and layout'),
    ).toBeTruthy()
    expect(
      view.getByText('Canvas').closest('button')?.querySelector('svg'),
    ).toBeTruthy()
    expect(
      view.getByText('Blog').closest('button')?.querySelector('svg'),
    ).toBeNull()
    expect(
      view.getByText('Canvas').closest('button')?.textContent,
    ).not.toContain('Exp')
    expect(view.getByText('Blog').closest('button')?.textContent).toContain(
      'Exp',
    )
  })

  it('keeps the palette open when a sub-item completes a path query', async () => {
    mockCommands = [
      {
        command: {
          commandId: 'canvas.add-image',
          label: 'Add Image',
          menuPath: 'Tools/Add Image',
          hasSubItems: true,
        },
        active: true,
        enabled: true,
      },
    ]
    mockGetSubItems.mockResolvedValue([
      {
        id: createSubItemQueryId('photos/'),
        label: '/photos/',
        description: 'Directory',
      },
    ])

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    fireEvent.click(view.getByText('Add Image'))
    await waitFor(() => expect(view.getByText('/photos/')).toBeTruthy())

    fireEvent.click(view.getByText('/photos/'))

    await waitFor(() => {
      expect(view.getByLabelText('Command search')).toHaveProperty(
        'value',
        'photos/',
      )
      expect(mockGetSubItems).toHaveBeenCalledWith(
        'canvas.add-image',
        'photos/',
        expect.any(AbortSignal),
      )
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('keeps f as a chord step at the root instead of starting a filter', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-file',
              binding: {
                case: 'sequence',
                value: { steps: ['Leader', 'F', 'O'] },
              },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    const input = view.getByLabelText('Command search')

    act(() => {
      fireEvent.keyDown(input, { key: 'f' })
    })

    expect(view.getByText('Chord mode')).toBeTruthy()
    expect(view.getByLabelText('Command search')).toHaveProperty('value', '')
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('enters filter mode from slash and from a non-matching printable key', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-file',
              binding: { case: 'sequence', value: { steps: ['Leader', 'F'] } },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const slashView = render(<CommandPalette />)
    act(() => paletteHandler?.())
    act(() => {
      fireEvent.keyDown(slashView.getByLabelText('Command search'), {
        key: '/',
      })
    })

    expect(slashView.getByText('Filter mode')).toBeTruthy()
    expect(slashView.getByLabelText('Command search')).toHaveProperty(
      'value',
      '',
    )
    slashView.unmount()

    const printableView = render(<CommandPalette />)
    act(() => paletteHandler?.())
    act(() => {
      fireEvent.keyDown(printableView.getByLabelText('Command search'), {
        key: 'z',
      })
    })

    expect(printableView.getByText('Filter mode')).toBeTruthy()
    expect(printableView.getByLabelText('Command search')).toHaveProperty(
      'value',
      'z',
    )
  })

  it('restores chord mode when Backspace empties the filter', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-file',
              binding: { case: 'sequence', value: { steps: ['Leader', 'F'] } },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    const input = view.getByLabelText('Command search')

    act(() => {
      fireEvent.keyDown(input, { key: 'z' })
    })
    expect(view.getByText('Filter mode')).toBeTruthy()

    act(() => {
      fireEvent.change(input, { target: { value: '' } })
    })

    expect(view.getByText('Chord mode')).toBeTruthy()
  })

  it('lets native select-all delete clear the whole filter query', () => {
    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    const input = view.getByLabelText('Command search') as HTMLInputElement

    act(() => {
      fireEvent.keyDown(input, { key: 'a' })
    })
    act(() => {
      fireEvent.change(input, { target: { value: 'abc' } })
    })
    expect(input.value).toBe('abc')
    expect(view.getByText('Filter mode')).toBeTruthy()

    // Select-all must not be intercepted by the palette, so the native input
    // performs the selection.
    const selectAllNotPrevented = fireEvent.keyDown(input, {
      key: 'a',
      metaKey: true,
    })
    expect(selectAllNotPrevented).toBe(true)

    // Backspace after select-all must not be intercepted; the old handler
    // preventDefaulted it and sliced one character, so the selection delete
    // removed only a single char instead of clearing the query.
    const backspaceNotPrevented = fireEvent.keyDown(input, { key: 'Backspace' })
    expect(backspaceNotPrevented).toBe(true)

    // The native selection delete empties the input.
    act(() => {
      fireEvent.change(input, { target: { value: '' } })
    })
    expect(input.value).toBe('')
    expect(view.getByText('Chord mode')).toBeTruthy()
  })

  it('waits for the next chord key before dispatching', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-file',
              binding: {
                case: 'sequence',
                value: { steps: ['Leader', 'F', 'O'] },
              },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())
    const input = view.getByLabelText('Command search')

    act(() => {
      fireEvent.keyDown(input, { key: 'f' })
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()

    act(() => {
      fireEvent.keyDown(input, { key: 'o' })
    })
    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.file.open')
  })
})
