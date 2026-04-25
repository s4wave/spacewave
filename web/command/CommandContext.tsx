import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  type ReactNode,
} from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResourcesContext } from '@aptre/bldr-sdk/hooks/ResourcesContext.js'
import type { Client as ResourceClient } from '@aptre/bldr-sdk/resource/client.js'
import type { Root } from '@s4wave/sdk/root'
import type { LookupMethod } from 'starpc'
import {
  CommandRegistryResourceServiceClient,
  type CommandRegistryResourceService,
} from '@s4wave/sdk/command/registry/registry_srpc.pb.js'
import {
  WatchCommandsRequest,
  WatchCommandsResponse,
  type CommandState,
} from '@s4wave/sdk/command/registry/registry.pb.js'

// SubItem is an item in a command's sub-item list.
export interface SubItem {
  id: string
  label: string
  description?: string
}

// SubItemsCallback provides sub-items for a command's palette sub-list.
export type SubItemsCallback = (
  query: string,
  signal: AbortSignal,
) => Promise<SubItem[]>

// OpenCommandHandler opens the command palette focused on a command.
// For commands with sub-items, this opens directly in sub-item mode.
type OpenCommandHandler = (commandId: string) => void

// CommandContextValue provides command registry state and operations.
interface CommandContextValue {
  // commands is the full registry from WatchCommands.
  commands: CommandState[]
  // invokeCommand sends a command invocation through the registry.
  invokeCommand: (commandId: string, args?: Record<string, string>) => void
  // openCommand opens the command palette focused on a command.
  // For commands with sub-items, this opens directly in sub-item mode.
  openCommand: (commandId: string) => void
  // service is the command registry service client.
  service: CommandRegistryResourceService | null
  // releaseResource releases a server-side resource by ID.
  // Used to unregister commands when useCommand unmounts.
  releaseResource: (resourceId: number) => void
  // attachResource attaches a client-side handler resource to the root client.
  attachResource: (
    label: string,
    mux: LookupMethod,
    signal?: AbortSignal,
  ) => Promise<{ resourceId: number; cleanup: () => void }>
  // getSubItems fetches sub-items through the registry.
  getSubItems: (
    commandId: string,
    query: string,
    signal: AbortSignal,
  ) => Promise<SubItem[]>
  // registerOpenCommand registers the palette's open handler.
  // Called by CommandPalette on mount. Returns a cleanup function.
  registerOpenCommand: (handler: OpenCommandHandler) => () => void
}

const CommandContext = createContext<CommandContextValue | null>(null)

// CommandProvider subscribes to the command registry and provides command
// state and invocation to descendant components.
export function CommandProvider({
  rootResource,
  children,
}: {
  rootResource: Resource<Root>
  children: ReactNode
}) {
  const root = rootResource.value
  const resources = useResourcesContext()
  const resourceClient = resources?.client ?? null
  const openCommandRef = useRef<OpenCommandHandler | null>(null)

  const watchFn = useCallback(
    (
      _: WatchCommandsRequest,
      signal: AbortSignal,
    ): AsyncIterable<WatchCommandsResponse> | null => {
      if (!root) return null
      const svc: CommandRegistryResourceService =
        new CommandRegistryResourceServiceClient(root.client)
      return svc.WatchCommands({}, signal)
    },
    [root],
  )

  const watchState = useWatchStateRpc<
    WatchCommandsResponse,
    WatchCommandsRequest
  >(watchFn, {}, WatchCommandsRequest.equals, WatchCommandsResponse.equals)

  const service = useMemo<CommandRegistryResourceService | null>(() => {
    if (!root) return null
    return new CommandRegistryResourceServiceClient(root.client)
  }, [root])

  const releaseResource = useCallback(
    (resourceId: number) => {
      if (!root || resourceId === 0 || root.resourceRef.released) return
      const ref = root.resourceRef.createRef(resourceId)
      ref.release()
    },
    [root],
  )

  const attachResource = useCallback(
    (
      label: string,
      mux: LookupMethod,
      signal?: AbortSignal,
    ): Promise<{ resourceId: number; cleanup: () => void }> => {
      const client: ResourceClient | null = resourceClient
      if (!client) {
        return Promise.reject(new Error('resource client unavailable'))
      }
      return client.attachResource(label, mux, signal)
    },
    [resourceClient],
  )

  const registerOpenCommand = useCallback(
    (handler: OpenCommandHandler): (() => void) => {
      openCommandRef.current = handler
      return () => {
        if (openCommandRef.current === handler) {
          openCommandRef.current = null
        }
      }
    },
    [],
  )

  const openCommand = useCallback((commandId: string) => {
    openCommandRef.current?.(commandId)
  }, [])

  const invokeCommand = useCallback(
    (commandId: string, args?: Record<string, string>) => {
      if (!service) return
      service.InvokeCommand({ commandId, args }).catch((err) => {
        console.error('InvokeCommand failed:', commandId, err)
      })
    },
    [service],
  )

  const getSubItems = useCallback(
    async (
      commandId: string,
      query: string,
      signal: AbortSignal,
    ): Promise<SubItem[]> => {
      if (!service) return []
      const resp = await service.GetSubItems({ commandId, query }, signal)
      return (resp.items ?? [])
        .filter((item) => !!item.id)
        .map((item) => ({
          id: item.id!,
          label: item.label ?? '',
          description: item.description || undefined,
        }))
    },
    [service],
  )

  const commands = useMemo<CommandState[]>(
    () => watchState?.commands ?? [],
    [watchState],
  )

  const value = useMemo<CommandContextValue>(
    () => ({
      commands,
      invokeCommand,
      openCommand,
      service,
      releaseResource,
      attachResource,
      getSubItems,
      registerOpenCommand,
    }),
    [
      commands,
      invokeCommand,
      openCommand,
      service,
      releaseResource,
      attachResource,
      getSubItems,
      registerOpenCommand,
    ],
  )

  return (
    <CommandContext.Provider value={value}>{children}</CommandContext.Provider>
  )
}

// emptyCommandContext is a no-op context value for components rendered
// outside a CommandProvider (e.g. in E2E tests without a live Root).
const emptyCommandContext: CommandContextValue = {
  commands: [],
  invokeCommand: () => {},
  openCommand: () => {},
  service: null,
  releaseResource: () => {},
  attachResource: () => Promise.resolve({ resourceId: 0, cleanup: () => {} }),
  getSubItems: () => Promise.resolve([]),
  registerOpenCommand: () => () => {},
}

// useCommandContext returns the command context value, or a no-op
// default if no CommandProvider is present in the tree.
export function useCommandContext(): CommandContextValue {
  return useContext(CommandContext) ?? emptyCommandContext
}

// useCommands returns the current command registry state.
export function useCommands(): CommandState[] {
  return useCommandContext().commands
}

// useInvokeCommand returns a function to invoke commands by ID.
export function useInvokeCommand(): CommandContextValue['invokeCommand'] {
  return useCommandContext().invokeCommand
}

// useOpenCommand returns a function to open the palette focused on a command.
export function useOpenCommand(): CommandContextValue['openCommand'] {
  return useCommandContext().openCommand
}

// useCommandService returns the command registry service client.
export function useCommandService(): CommandRegistryResourceService | null {
  return useCommandContext().service
}
