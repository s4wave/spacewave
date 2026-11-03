import type { StyleXStyles } from '@stylexjs/stylex'
import { useCallback, useMemo, createElement } from 'react'

import type { StaticConfigTypeRegistration } from './configtype.js'
import { useAllConfigTypes } from './ConfigTypeRegistryContext.js'

// ConfigEditorResult is the result of useConfigEditor.
export interface ConfigEditorResult {
  // render applies caller styles at the registered editor's physical root.
  render: (sx?: StyleXStyles) => React.ReactElement | null
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
  // Resolve the registration and its optional protobuf codec.
  const registrations = useAllConfigTypes()
  const reg = useMemo(
    () => registrations.find((r) => r.configId === configId),
    [registrations, configId],
  )
  const messageType = reg?.messageType as BinaryMessageType | undefined

  // Decode the controlled value without mutating the stored bytes.
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

  // Encode edits through the same registration that decoded the value.
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

  // Render styles at the registered component rather than at an added wrapper.
  const render = useCallback(
    (sx?: StyleXStyles) => {
      if (!reg || !value) return null
      return createElement(reg.component as never, { value, onValueChange, sx })
    },
    [reg, value, onValueChange],
  )

  return { render, registration: reg, value }
}
