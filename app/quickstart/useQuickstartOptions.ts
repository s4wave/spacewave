import React, { createContext, use, useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useDynamicRegistrations } from '@s4wave/web/hooks/useDynamicRegistrations.js'
import type { Root } from '@s4wave/sdk/root'
import { QuickstartRegistryResourceServiceClient } from '@s4wave/sdk/quickstart/registry/registry_srpc.pb.js'
import {
  WatchQuickstartsRequest,
  WatchQuickstartsResponse,
} from '@s4wave/sdk/quickstart/registry/registry.pb.js'

import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import {
  getVisibleQuickstartOptions,
  type QuickstartOption,
} from './options.js'
import { mergeQuickstartOptions } from './dynamic-options.js'

const QuickstartOptionsContext = createContext<QuickstartOption[]>(
  getVisibleQuickstartOptions(),
)

interface QuickstartOptionsProviderProps {
  rootResource: Resource<Root>
  children: React.ReactNode
}

// QuickstartOptionsProvider owns the dynamic Quickstart watch for the app tree.
export function QuickstartOptionsProvider({
  rootResource,
  children,
}: QuickstartOptionsProviderProps) {
  const quickstartOptions = useMergedQuickstartOptions(rootResource)
  return React.createElement(
    QuickstartOptionsContext.Provider,
    { value: quickstartOptions },
    children,
  )
}

// useVisibleQuickstartOptions returns static plus dynamic app Quickstarts.
export function useVisibleQuickstartOptions(): QuickstartOption[] {
  return use(QuickstartOptionsContext)
}

const quickstartCreateStream = (
  root: Root,
  _req: WatchQuickstartsRequest,
  signal: AbortSignal,
) =>
  new QuickstartRegistryResourceServiceClient(root.client).WatchQuickstarts(
    {},
    signal,
  )

const quickstartGetRegs = (resp: WatchQuickstartsResponse | null) =>
  resp?.registrations ?? []

const passRegistration = <T>(reg: T): T => reg

function useMergedQuickstartOptions(
  rootResource: Resource<Root>,
): QuickstartOption[] {
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const dynamicRegistrations = useDynamicRegistrations(
    rootResource.value,
    quickstartCreateStream,
    {},
    WatchQuickstartsRequest.equals,
    WatchQuickstartsResponse.equals,
    quickstartGetRegs,
    passRegistration,
  )
  const staticOptions = useMemo(
    () => getVisibleQuickstartOptions(experimentalCreatorsEnabled),
    [experimentalCreatorsEnabled],
  )
  return useMemo(
    () =>
      mergeQuickstartOptions(
        staticOptions,
        dynamicRegistrations,
        experimentalCreatorsEnabled,
      ),
    [dynamicRegistrations, experimentalCreatorsEnabled, staticOptions],
  )
}
