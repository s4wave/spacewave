import type { ComponentType } from 'react'
import type { Message, MessageType } from '@aptre/protobuf-es-lite'

// ConfigEditorProps are the props passed to config editor components.
// Config editors are controlled components: the parent owns the state.
export interface ConfigEditorProps<T> {
  // value is the current config protobuf message.
  value: T
  // onValueChange receives a clone with changes applied.
  // The component MUST NOT mutate the input value.
  onValueChange: (newValue: T) => void
}

// StaticConfigTypeRegistration describes a config type editor registration.
// Static (compiled-in) registrations provide messageType for automatic
// bytes<->typed conversion; the editor receives ConfigEditorProps<T>.
// Dynamic (plugin SRPC) registrations omit messageType; the editor receives
// ConfigEditorProps<Uint8Array> and handles its own proto serialization.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export interface StaticConfigTypeRegistration<T extends Message<T> = any> {
  // configId is the config type identifier (e.g. "forge/task", "git/repo").
  configId: string
  // displayName is the human-readable name shown in the UI.
  displayName: string
  // category groups the config type in the UI (e.g. "Forge", "Code").
  category?: string
  // messageType is the protobuf MessageType for bytes<->typed conversion.
  // Optional for dynamic registrations that handle serialization internally.
  messageType?: MessageType<T>
  // component is the React component that edits this config type.
  component: ComponentType<ConfigEditorProps<T>>
}
