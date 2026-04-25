import { useEffect, useEffectEvent, useState } from 'react'
import { createHandler } from 'starpc'
import { newResourceMux } from '@aptre/bldr-sdk/resource/server/index.js'

import {
  useCommandContext,
  type SubItem,
  type SubItemsCallback,
} from './CommandContext.js'
import {
  CommandHandlerServiceDefinition,
  type CommandHandlerService,
} from '@s4wave/sdk/command/registry/registry_srpc.pb.js'

// UseCommandOpts configures a command registration.
interface UseCommandOpts {
  // commandId is the unique reverse-domain command identifier.
  commandId: string
  // label is the human-readable display name.
  label: string
  // menuPath is the menu placement path (e.g. "File/Save").
  menuPath?: string
  // keybinding is the default key combination (e.g. "CmdOrCtrl+S").
  keybinding?: string
  // menuGroup controls separator placement within a menu level.
  menuGroup?: number
  // menuOrder controls ordering within a group.
  menuOrder?: number
  // icon is an optional icon identifier.
  icon?: string
  // description is an optional longer description for the palette.
  description?: string
  // hasSubItems indicates the command has a sub-item list in the palette.
  hasSubItems?: boolean
  // subItems provides the sub-item list callback for the palette.
  // Only used when hasSubItems is true.
  subItems?: SubItemsCallback
  // handler is called when the command is invoked.
  handler: (args: Record<string, string>) => void
  // active controls whether the command is active in the current UI tree.
  // Defaults to true.
  active?: boolean
  // enabled controls whether the command is enabled (clickable).
  // Disabled commands appear dimmed in palette and menu bar. Defaults to true.
  enabled?: boolean
}

// useCommand registers a command with the command registry and sets up
// a client-side handler. Manages registration, activation, and cleanup.
export function useCommand(opts: UseCommandOpts): void {
  const { service, releaseResource, attachResource } = useCommandContext()
  const [registrationResourceId, setRegistrationResourceId] = useState(0)
  const handleCommand = useEffectEvent((args: Record<string, string>) => {
    opts.handler(args)
  })
  const getSubItems = useEffectEvent(
    async (query: string, signal: AbortSignal): Promise<SubItem[]> => {
      const items = await opts.subItems?.(query, signal)
      return items ?? []
    },
  )

  const active = opts.active ?? true
  const enabled = opts.enabled ?? true

  useEffect(() => {
    if (!service) return

    const abort = new AbortController()
    let registrationId = 0
    let detachHandlerResource: (() => void) | null = null

    const handlerService: CommandHandlerService = {
      GetSubItems: async (req, signal) => {
        const items = await getSubItems(req.query ?? '', signal ?? abort.signal)
        return { items }
      },
      HandleCommand: (req) => {
        handleCommand(req.args ?? {})
        return Promise.resolve({})
      },
    }

    const handlerMux = newResourceMux(
      createHandler(CommandHandlerServiceDefinition, handlerService),
    )

    attachResource(
      `command:${opts.commandId}`,
      handlerMux.lookupMethod,
      abort.signal,
    )
      .then(async ({ resourceId, cleanup }) => {
        detachHandlerResource = cleanup
        const resp = await service.RegisterCommand(
          {
            command: {
              commandId: opts.commandId,
              label: opts.label,
              keybinding: opts.keybinding,
              menuPath: opts.menuPath,
              menuGroup: opts.menuGroup,
              menuOrder: opts.menuOrder,
              icon: opts.icon,
              description: opts.description,
              hasSubItems: opts.hasSubItems,
            },
            handlerResourceId: resourceId,
          },
          abort.signal,
        )
        registrationId = resp.resourceId ?? 0
        setRegistrationResourceId(registrationId)
      })
      .catch((err) => {
        if (!abort.signal.aborted) {
          console.error('RegisterCommand failed:', opts.commandId, err)
        }
      })

    return () => {
      abort.abort()
      detachHandlerResource?.()
      setRegistrationResourceId((current) =>
        current === registrationId ? 0 : current,
      )
      if (registrationId !== 0) {
        releaseResource(registrationId)
      }
    }
  }, [
    attachResource,
    service,
    releaseResource,
    opts.commandId,
    opts.label,
    opts.keybinding,
    opts.menuPath,
    opts.menuGroup,
    opts.menuOrder,
    opts.icon,
    opts.description,
    opts.hasSubItems,
  ])

  useEffect(() => {
    if (!service || registrationResourceId === 0) return

    const abort = new AbortController()
    service
      .SetActive({ resourceId: registrationResourceId, active }, abort.signal)
      .catch((err) => {
        if (!abort.signal.aborted) {
          console.error('SetActive failed:', opts.commandId, err)
        }
      })
    return () => {
      abort.abort()
    }
  }, [service, registrationResourceId, opts.commandId, active])

  useEffect(() => {
    if (!service || registrationResourceId === 0) return

    const abort = new AbortController()
    service
      .SetEnabled({ resourceId: registrationResourceId, enabled }, abort.signal)
      .catch((err) => {
        if (!abort.signal.aborted) {
          console.error('SetEnabled failed:', opts.commandId, err)
        }
      })
    return () => {
      abort.abort()
    }
  }, [service, registrationResourceId, opts.commandId, enabled])
}
