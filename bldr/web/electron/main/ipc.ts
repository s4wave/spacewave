import { MessageChannelMain, MessagePortMain } from 'electron'

// convert MessagePort to a MessagePortMain.
export function messagePortToMessagePortMain(
  port: MessagePort,
): MessagePortMain {
  const channel = new MessageChannelMain()
  channel.port1.on('message', (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortMainToMessagePort(port))
      port.postMessage(ev.data, ports)
    } else {
      port.postMessage(ev.data)
    }
  })
  port.onmessage = (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortToMessagePortMain(port))
      channel.port1.postMessage(ev.data, ports)
    } else {
      channel.port1.postMessage(ev.data)
    }
  }
  port.start()
  channel.port1.start()
  return channel.port2
}

// convert MessagePortMain to a MessagePort.
export function messagePortMainToMessagePort(
  portMain: MessagePortMain,
): MessagePort {
  const channel = new MessageChannel()
  channel.port1.onmessage = (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortToMessagePortMain(port))
      portMain.postMessage(ev.data, ports)
    } else {
      portMain.postMessage(ev.data)
    }
  }
  portMain.on('message', (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortMainToMessagePort(port))
      channel.port1.postMessage(ev.data, ports)
    } else {
      channel.port1.postMessage(ev.data)
    }
  })
  portMain.start()
  channel.port1.start()
  return channel.port2
}
