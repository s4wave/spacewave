import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'

import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'

import { SelectAccountCommand } from './SelectAccountCommand.js'

interface RegisteredCommand {
  commandId: string
  label: string
  description?: string
  menuPath?: string
  menuGroup?: number
  menuOrder?: number
  active?: boolean
  hasSubItems?: boolean
  subItems?: SubItemsCallback
  handler: (args: Record<string, string>) => void
}

const h = vi.hoisted(() => ({
  commands: [] as RegisteredCommand[],
  path: '/sessions',
  sessions: [{ sessionIndex: 1 }, { sessionIndex: 2 }],
  navigate: vi.fn(),
  getSessionMetadata: vi.fn(),
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    h.commands.push(opts)
  },
}))

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => ({ value: { sessions: h.sessions } }),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => ({
    value: { getSessionMetadata: h.getSessionMetadata },
  }),
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => h.navigate,
  usePath: () => h.path,
}))

function findSelectAccountCommand(): RegisteredCommand {
  const command = h.commands.find(
    (item) => item.commandId === 'spacewave.session.switch',
  )
  if (!command) throw new Error('Select Account command was not registered')
  return command
}

describe('SelectAccountCommand', () => {
  beforeEach(() => {
    h.commands.length = 0
    h.path = '/sessions'
    h.sessions = [{ sessionIndex: 1 }, { sessionIndex: 2 }]
    h.navigate.mockReset()
    h.getSessionMetadata.mockReset()
    h.getSessionMetadata.mockImplementation((sessionIndex: number) =>
      Promise.resolve({
        metadata:
          sessionIndex === 1
            ? {
                displayName: 'Alice',
                providerId: 'spacewave',
                providerDisplayName: 'Cloud',
                cloudEntityId: 'alice@example.com',
              }
            : {
                displayName: 'Bob',
                providerId: 'local',
                providerDisplayName: 'Local',
              },
      }),
    )
  })

  afterEach(() => cleanup())

  it('opens a filtered account submenu and selects through the shared route', async () => {
    h.path = '/u/1/'
    render(<SelectAccountCommand />)

    const command = findSelectAccountCommand()
    expect(command).toMatchObject({
      label: 'Select Account',
      description: 'Select an account to use',
      menuPath: 'File/Select Account',
      menuGroup: 80,
      menuOrder: 2,
      active: true,
      hasSubItems: true,
    })

    const items = await command.subItems?.('', new AbortController().signal)
    expect(items).toEqual([
      {
        id: '1',
        label: 'Alice',
        description: 'Cloud · alice@example.com',
        iconName: 'LuCheck',
      },
      {
        id: '2',
        label: 'Bob',
        description: 'Local',
        iconName: 'LuUser',
      },
    ])

    const filteredItems = await command.subItems?.(
      'bob',
      new AbortController().signal,
    )
    expect(filteredItems).toEqual([
      {
        id: '2',
        label: 'Bob',
        description: 'Local',
        iconName: 'LuUser',
      },
    ])

    command.handler({ subItemId: '2' })
    expect(h.navigate).toHaveBeenCalledWith({ path: '/u/2/' })
  })

  it('hides on an account view when no alternate account exists', () => {
    h.path = '/u/1/'
    h.sessions = [{ sessionIndex: 1 }]

    render(<SelectAccountCommand />)

    expect(findSelectAccountCommand()).toMatchObject({ active: false })
  })

  it('stays available outside account views with one account', () => {
    h.path = '/sessions'
    h.sessions = [{ sessionIndex: 1 }]

    render(<SelectAccountCommand />)

    expect(findSelectAccountCommand()).toMatchObject({ active: true })
  })
})
