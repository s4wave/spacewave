import { useCallback, useMemo } from 'react'

import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSelectAccount } from '@s4wave/app/hooks/useSelectAccount.js'
import type {
  SubItem,
  SubItemsCallback,
} from '@s4wave/web/command/CommandContext.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { getAppPath } from '@s4wave/web/router/app-path.js'

import { accountDescription, accountTitle } from './account-presentation.js'

// SelectAccountCommand registers the global account-selection palette command.
export function SelectAccountCommand() {
  const root = useRootResource().value
  const sessionList = useSessionList().value?.sessions
  const sessions = useMemo(() => sessionList ?? [], [sessionList])
  const selectAccount = useSelectAccount()
  const path = getAppPath()
  const accountPathMatch = /^\/u\/(\d+)(?:\/|$)/.exec(path)
  const currentSessionIndex = accountPathMatch
    ? Number(accountPathMatch[1])
    : null
  const isAccountView = path === '/u' || path.startsWith('/u/')
  const availableAccountCount = sessions.reduce(
    (count, session) => count + (session.sessionIndex ? 1 : 0),
    0,
  )

  const subItems: SubItemsCallback = useCallback(
    async (query: string, signal: AbortSignal) => {
      const normalizedQuery = query.trim().toLowerCase()
      const items = await Promise.all(
        sessions.map(async (session): Promise<SubItem | null> => {
          const sessionIndex = session.sessionIndex
          if (!sessionIndex) return null

          const metadata = root
            ? (await root.getSessionMetadata(sessionIndex, signal)).metadata
            : null
          const label = accountTitle(metadata, sessionIndex)
          const description = accountDescription(metadata, sessionIndex)
          if (
            normalizedQuery &&
            !label.toLowerCase().includes(normalizedQuery) &&
            !description.toLowerCase().includes(normalizedQuery)
          ) {
            return null
          }

          return {
            id: String(sessionIndex),
            label,
            description,
            iconName:
              sessionIndex === currentSessionIndex ? 'LuCheck' : 'LuUser',
          }
        }),
      )
      return items.filter((item): item is SubItem => item !== null)
    },
    [currentSessionIndex, root, sessions],
  )

  useCommand({
    commandId: 'spacewave.session.switch',
    label: 'Select Account',
    description: 'Select an account to use',
    active: !isAccountView || availableAccountCount > 1,
    hasSubItems: true,
    subItems,
    handler: useCallback(
      (args: Record<string, string>) => {
        const sessionIndex = Number(args.subItemId)
        if (Number.isSafeInteger(sessionIndex) && sessionIndex > 0) {
          selectAccount(sessionIndex)
        }
      },
      [selectAccount],
    ),
  })

  return null
}
