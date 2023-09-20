import { MessageDefinition } from 'starpc'

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
export type ProtoRenderFunc = (
  props: IRenderProtoProps,
) => React.ReactNode | JSX.Element | undefined

// renderProto wraps a render function with parsing a protobuf props object.
export function renderProto<T>(
  def: MessageDefinition<T>,
  render: (props: T) => React.ReactNode | JSX.Element | undefined,
): ProtoRenderFunc {
  return (props: IRenderProtoProps) => {
    return render(def.decode(props.componentProps || new Uint8Array(0)))
  }
}
