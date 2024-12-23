import React, { useMemo } from 'react'
import { Message, MessageType } from '@aptre/protobuf-es-lite'
import { useMemoUint8Array } from './hooks.js'

// IRenderProtoProps are props passed to an imported ReactComponent.
export interface IRenderProtoProps {
  // componentProps is an optional props message to the component.
  componentProps?: Uint8Array
}

// ProtoComponentType is the type the loaded component should implement.
export type ProtoComponentType = React.ComponentType<IRenderProtoProps>

// LoadedProtoComponent is a lazy-loaded React component.
export type LoadedProtoComponent = React.LazyExoticComponent<ProtoComponentType>

// ProtoRenderFunc is a valid render function accepting a protobuf message.
export type ProtoRenderFunc = React.FC<IRenderProtoProps>

// renderProto wraps a render function with parsing a protobuf props object.
export function renderProto<T extends Message<T>>(
  def: MessageType<T>,
  render: React.FC<T>,
): ProtoRenderFunc {
  return (props: IRenderProtoProps) => {
    const memoComponentProps = useMemoUint8Array(props.componentProps ?? null)
    return useMemo(
      () => render(def.fromBinary(memoComponentProps)),
      [memoComponentProps],
    )
  }
}
