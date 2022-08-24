// MessagePortBridgeCallback is called with a data packet.
export type MessagePortBridgeCallback<T> = (
  data: T,
  ports?: readonly MessagePortBridge<unknown>[]
) => void

// MessagePortBridge is a message port emulated with callback functions.
// Type parameters are <Incoming, Outgoing>
export interface MessagePortBridge<I, O = I> {
  // start sets the message callback function and starts messages.
  start: (cb: MessagePortBridgeCallback<O>) => void
  // write writes data to the remote.
  write: (data: I, ports?: readonly MessagePortBridge<unknown>[]) => void
  // close closes the port.
  close: () => void
}

// messagePortBridgeToMessagePort converts a MessagePortBridge into a MessagePort.
export function messagePortBridgeToMessagePort<T>(
  bridge: MessagePortBridge<T>
): MessagePort {
  const channel = new MessageChannel()
  const localPort = channel.port1
  const bridgePort = channel.port2
  bridge.start((data: T, ports?: readonly MessagePortBridge<unknown>[]) => {
    if (ports && ports.length) {
      const bridgePorts = ports.map((port) =>
        messagePortBridgeToMessagePort(port)
      )
      bridgePort.postMessage(data, bridgePorts)
    } else {
      bridgePort.postMessage(data)
    }
  })
  bridgePort.onmessage = (ev) => {
    const { data, ports } = ev
    if (ports && ports.length) {
      const bridgePorts = ports.map((port) =>
        messagePortToMessagePortBridge(port)
      )
      bridge.write(data, bridgePorts)
    } else {
      bridge.write(data)
    }
  }
  bridgePort.start()
  return localPort
}

// messagePortToMessagePortBridge converts a MessagePort into a MessagePortBridge.
export function messagePortToMessagePortBridge<T>(
  port: MessagePort
): MessagePortBridge<T> {
  return {
    start: (cb: MessagePortBridgeCallback<T>) => {
      port.onmessage = (ev) => {
        if (ev.ports && ev.ports.length) {
          const bridgePorts = ev.ports.map((port) =>
            messagePortToMessagePortBridge(port)
          )
          cb(ev.data, bridgePorts)
        } else {
          cb(ev.data)
        }
      }
      port.start()
    },
    write: (data: T, ports?: readonly MessagePortBridge<unknown>[]) => {
      if (ports && ports.length) {
        const bridgePorts = ports.map((port) =>
          messagePortBridgeToMessagePort(port)
        )
        port.postMessage(data, bridgePorts)
      } else {
        port.postMessage(data)
      }
    },
    close: () => {
      port.close()
    },
  }
}
