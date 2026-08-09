import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'

import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'

import { ShellMenuBar } from './ShellMenuBar.js'

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

const mockUseCommands = vi.fn()
const mockInvokeCommand = vi.fn()
const mockOpenCommand = vi.fn()

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...classes: Array<string | false | null | undefined>) =>
    classes.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/web/images/AppLogo.js', () => ({
  AppLogo: ({ className }: { className?: string }) => (
    <div className={className}>logo</div>
  ),
}))

vi.mock('@s4wave/web/command/index.js', () => ({
  useCommands: () => {
    const commands: unknown = mockUseCommands()
    return commands
  },
  useInvokeCommand: () => mockInvokeCommand,
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/command/CommandPalette.js', () => ({
  formatKeybindingHint: (bindings: string[]) => bindings.join(' / '),
}))

vi.mock('@s4wave/web/ui/Menubar.js', () => ({
  Menubar: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarItem: ({
    children,
    onSelect,
    disabled,
  }: {
    children: React.ReactNode
    onSelect?: () => void
    disabled?: boolean
  }) => (
    <button disabled={disabled} onClick={onSelect} type="button">
      {children}
    </button>
  ),
  MenubarMenu: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSeparator: () => <hr />,
  MenubarShortcut: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  MenubarSub: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSubContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSubTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarTrigger: ({
    children,
  }: {
    children: React.ReactNode
    asChild?: boolean
  }) => <div>{children}</div>,
}))

describe('ShellMenuBar', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('opens the palette for menu commands with sub-items', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.go-to-space',
          label: 'Go to Space',
          menuPath: 'View/Go to Space',
          hasSubItems: true,
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(<ShellMenuBar />)

    fireEvent.click(view.getByRole('button', { name: 'Go to Space' }))

    expect(mockOpenCommand).toHaveBeenCalledWith('spacewave.nav.go-to-space')
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('opens the palette logo through the local palette owner', () => {
    mockUseCommands.mockReturnValue([])

    const view = render(<ShellMenuBar />)

    fireEvent.click(view.getByRole('button', { name: 'Open command palette' }))

    expect(mockOpenCommand).toHaveBeenCalledWith('spacewave.view.palette')
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('invokes regular menu commands directly', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.home',
          label: 'Go Home',
          menuPath: 'View/Go Home',
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(<ShellMenuBar />)

    fireEvent.click(view.getByRole('button', { name: 'Go Home' }))

    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.nav.home')
    expect(mockOpenCommand).not.toHaveBeenCalled()
  })

  it('shows only effective combo hints for menu commands', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.home',
          label: 'Go Home',
          menuPath: 'View/Go Home',
          defaultBindings: [
            {
              id: 'home-combo',
              binding: { case: 'combo', value: { combo: 'Ctrl+H' } },
              surface: CommandSurface.WEB,
            },
            {
              id: 'home-sequence',
              binding: { case: 'sequence', value: { steps: ['Leader', 'H'] } },
              surface: CommandSurface.WEB,
            },
          ],
        },
        active: true,
        enabled: true,
      },
      {
        command: {
          commandId: 'spacewave.nav.search',
          label: 'Search',
          menuPath: 'View/Search',
          defaultBindings: [
            {
              id: 'search-sequence',
              binding: { case: 'sequence', value: { steps: ['Leader', 'S'] } },
              surface: CommandSurface.WEB,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(<ShellMenuBar />)

    expect(view.getByText('Ctrl+H')).toBeTruthy()
    expect(view.queryByText(/Leader/)).toBeNull()
  })
  it('keeps File and Go menus grouped by command subject', () => {
    mockUseCommands.mockReturnValue([
      ...[
        [
          'spacewave.create-object',
          'Create Object',
          'File/Create Object',
          10,
          0,
        ],
        ['spacewave.file.new-file', 'New File', 'File/New File', 10, 1],
        ['spacewave.file.new-folder', 'New Folder', 'File/New Folder', 10, 2],
        ['spacewave.file.upload', 'Upload', 'File/Upload', 10, 3],
        ['spacewave.file.open', 'Open Selected', 'File/Open Selected', 20, 1],
        [
          'spacewave.session.join-space',
          'Join Space',
          'File/Join Space',
          20,
          2,
        ],
        ['spacewave.file.rename', 'Rename', 'File/Rename', 30, 1],
        ['spacewave.file.download', 'Download', 'File/Download', 30, 2],
        [
          'spacewave.file.export-object',
          'Export Object',
          'File/Export Object',
          30,
          3,
        ],
        [
          'spacewave.file.rename-space',
          'Rename Space',
          'File/Rename Space',
          40,
          1,
        ],
        ['spacewave.share-space', 'Share Space', 'File/Share Space', 40, 2],
        ['spacewave.file.export', 'Export Space', 'File/Export Space', 40, 3],
        [
          'spacewave.file.close-space',
          'Close Space',
          'File/Close Space',
          50,
          1,
        ],
        ['spacewave.nav.back', 'Navigate Back', 'Go/Navigate Back', 10, 1],
        [
          'spacewave.nav.forward',
          'Navigate Forward',
          'Go/Navigate Forward',
          10,
          2,
        ],
        ['spacewave.nav.up', 'Navigate Up', 'Go/Navigate Up', 10, 3],
      ].map(([commandId, label, menuPath, menuGroup, menuOrder]) => ({
        command: { commandId, label, menuPath, menuGroup, menuOrder },
        active: true,
        enabled: true,
      })),
    ])

    const view = render(<ShellMenuBar />)
    const getMenuContent = (name: string) => {
      const trigger = view.getByRole('button', { name })
      const menu = trigger.parentElement?.parentElement
      return menu?.children[1] as HTMLElement
    }

    const fileContent = getMenuContent('File')
    expect(
      Array.from(fileContent.querySelectorAll('button')).map(
        (button) => button.textContent,
      ),
    ).toEqual([
      'Create Object',
      'New File',
      'New Folder',
      'Upload',
      'Open Selected',
      'Join Space',
      'Rename',
      'Download',
      'Export Object',
      'Rename Space',
      'Share Space',
      'Export Space',
      'Close Space',
    ])
    expect(fileContent.querySelectorAll('hr')).toHaveLength(4)

    const goContent = getMenuContent('Go')
    expect(
      Array.from(goContent.querySelectorAll('button')).map(
        (button) => button.textContent,
      ),
    ).toEqual(['Navigate Back', 'Navigate Forward', 'Navigate Up'])
    expect(goContent.querySelectorAll('hr')).toHaveLength(0)

    const fileTrigger = view.getByRole('button', { name: 'File' })
    const goTrigger = view.getByRole('button', { name: 'Go' })
    expect(
      fileTrigger.compareDocumentPosition(goTrigger) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })
})
