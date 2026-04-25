import { useCallback, useMemo, createElement } from 'react'

import type { StaticConfigTypeRegistration } from './configtype.js'
import { useAllConfigTypes } from './ConfigTypeRegistryContext.js'

// ConfigEditorResult is the result of useConfigEditor.
export interface ConfigEditorResult {
  // element is the rendered config editor, or null if no editor is registered.
  element: React.ReactElement | null
  // registration is the matched registration, or undefined if not found.
  registration: StaticConfigTypeRegistration | undefined
  // value is the decoded config message, or undefined if not available.
  value: unknown
}

interface BinaryMessageType {
  fromBinary(data: Uint8Array): unknown
  toBinary(value: unknown): Uint8Array
}

// useConfigEditor looks up a config type registration by ID and renders the
// editor component. For static registrations with messageType, handles
// bytes<->typed conversion automatically. For dynamic registrations without
// messageType, passes raw bytes to the component.
export function useConfigEditor(
  configId: string | undefined,
  configData: Uint8Array | undefined,
  onConfigDataChange: (data: Uint8Array) => void,
): ConfigEditorResult {
  const registrations = useAllConfigTypes()
  const reg = useMemo(
    () => registrations.find((r) => r.configId === configId),
    [registrations, configId],
  )
  const messageType = reg?.messageType as BinaryMessageType | undefined

  const value = useMemo(() => {
    if (!reg) return undefined
    // Dynamic registrations without messageType pass raw bytes.
    if (!messageType) return configData ?? new Uint8Array()
    if (!configData?.length) {
      try {
        return messageType.fromBinary(new Uint8Array())
      } catch {
        return undefined
      }
    }
    try {
      return messageType.fromBinary(configData)
    } catch {
      return undefined
    }
  }, [reg, messageType, configData])

  const onValueChange = useCallback(
    (newValue: unknown) => {
      if (!reg) return
      if (!messageType) {
        onConfigDataChange(newValue as Uint8Array)
        return
      }
      onConfigDataChange(messageType.toBinary(newValue))
    },
    [reg, messageType, onConfigDataChange],
  )

  const element = useMemo(() => {
    if (!reg || !value) return null
    return createElement(reg.component as never, { value, onValueChange })
  }, [reg, value, onValueChange])

  return { element, registration: reg, value }
}
