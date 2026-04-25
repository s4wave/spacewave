import { createContext, useContext, useMemo } from 'react'
import type { Root } from '@s4wave/sdk/root'
import { RootContext } from '../contexts/contexts.js'
import React from 'react'

import { ConfigTypeRegistryResourceServiceClient } from '@s4wave/sdk/configtype/registry/registry_srpc.pb.js'
import {
  WatchConfigTypesRequest,
  WatchConfigTypesResponse,
  type ConfigTypeRegistration,
} from '@s4wave/sdk/configtype/registry/registry.pb.js'
import type { StaticConfigTypeRegistration } from './configtype.js'
import { useDynamicRegistrations } from '../hooks/useDynamicRegistrations.js'

const emptyRegistrations: StaticConfigTypeRegistration[] = []

const ConfigTypeRegistryContext =
  createContext<StaticConfigTypeRegistration[]>(emptyRegistrations)

// ConfigTypeRegistryProviderProps are the props for ConfigTypeRegistryProvider.
interface ConfigTypeRegistryProviderProps {
  staticConfigTypes: StaticConfigTypeRegistration[]
  children: React.ReactNode
}

// ConfigTypeRegistryProvider supplies a static config type list via context.
export function ConfigTypeRegistryProvider({
  staticConfigTypes,
  children,
}: ConfigTypeRegistryProviderProps) {
  return (
    <ConfigTypeRegistryContext.Provider value={staticConfigTypes}>
      {children}
    </ConfigTypeRegistryContext.Provider>
  )
}

// useStaticConfigTypes returns the static config types from context.
export function useStaticConfigTypes(): StaticConfigTypeRegistration[] {
  return useContext(ConfigTypeRegistryContext)
}

// useAllConfigTypes returns all config types: static from context + dynamic from RPC.
export function useAllConfigTypes(): StaticConfigTypeRegistration[] {
  const staticTypes = useStaticConfigTypes()
  const rootResource = RootContext.useContext()
  const dynamicTypes = useDynamicRegistrations(
    rootResource.value,
    configTypeCreateStream,
    {},
    WatchConfigTypesRequest.equals,
    WatchConfigTypesResponse.equals,
    configTypeGetRegs,
    registrationToConfigType,
  )
  return useMemo(
    () => [...staticTypes, ...dynamicTypes],
    [staticTypes, dynamicTypes],
  )
}

// useConfigType looks up a config type registration by config ID.
export function useConfigType(
  configId: string | undefined,
): StaticConfigTypeRegistration | undefined {
  const allTypes = useAllConfigTypes()
  return useMemo(
    () => allTypes.find((r) => r.configId === configId),
    [allTypes, configId],
  )
}

const configTypeCreateStream = (
  root: Root,
  _req: WatchConfigTypesRequest,
  signal: AbortSignal,
) =>
  new ConfigTypeRegistryResourceServiceClient(root.client).WatchConfigTypes(
    {},
    signal,
  )

const configTypeGetRegs = (resp: WatchConfigTypesResponse | null) =>
  resp?.registrations ?? []

// registrationToConfigType converts a dynamic ConfigTypeRegistration to a
// StaticConfigTypeRegistration with a React.lazy loaded component.
function registrationToConfigType(
  reg: ConfigTypeRegistration,
): StaticConfigTypeRegistration | null {
  const configId = reg.configId
  const scriptPath = reg.scriptPath
  if (!configId || !scriptPath) return null

  return {
    configId,
    displayName: reg.displayName || configId,
    category: reg.category || undefined,
    component: React.lazy(() => import(/* @vite-ignore */ scriptPath)),
  }
}
