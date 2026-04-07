// cross-tab-broker.ts is a stateless cross-tab channel broker for ServiceWorker.
//
// When a tab sends "hello", the broker calls clients.matchAll() to find all
// open tabs, creates a MessageChannel for each peer pair, and transfers one
// port to each. After setup, the SW is out of the data path - tabs communicate
// directly via the transferred MessagePorts. No in-memory state in the SW.
//
// Protocol:
//   Tab -> SW:  { crossTab: "hello" }
//   Tab -> SW:  { crossTab: "goodbye" }
//   SW -> Tab:  { crossTab: "direct-port", peerId: string } + [MessagePort]
//   SW -> Tab:  { crossTab: "peer-gone", peerId: string }

// CrossTabClientMessage is sent from a tab to the ServiceWorker.
export interface CrossTabClientMessage {
  crossTab: 'hello' | 'goodbye'
}

// CrossTabBrokerMessage is sent from the ServiceWorker to a tab.
export type CrossTabBrokerMessage =
  | { crossTab: 'direct-port'; peerId: string }
  | { crossTab: 'peer-gone'; peerId: string }

// isCrossTabMessage checks if a message is a cross-tab broker message.
export function isCrossTabMessage(data: unknown): data is CrossTabClientMessage {
  if (typeof data !== 'object' || data === null) return false
  const msg = data as Record<string, unknown>
  return msg.crossTab === 'hello' || msg.crossTab === 'goodbye'
}

// handleCrossTabMessage handles a cross-tab broker message in the ServiceWorker.
// Stateless: uses clients.matchAll() as source of truth. SW can terminate and
// restart at any time without losing state.
export async function handleCrossTabMessage(
  clients: Clients,
  senderId: string,
  msg: CrossTabClientMessage,
): Promise<void> {
  if (msg.crossTab === 'hello') {
    const allClients = await clients.matchAll({ type: 'window' })

    // Create a direct channel between the new tab and every existing tab.
    for (const client of allClients) {
      if (client.id === senderId) continue
      const channel = new MessageChannel()
      client.postMessage(
        { crossTab: 'direct-port', peerId: senderId } satisfies CrossTabBrokerMessage,
        [channel.port1],
      )
      const sender = allClients.find((c) => c.id === senderId)
      if (sender) {
        sender.postMessage(
          { crossTab: 'direct-port', peerId: client.id } satisfies CrossTabBrokerMessage,
          [channel.port2],
        )
      }
    }
  } else if (msg.crossTab === 'goodbye') {
    const allClients = await clients.matchAll({ type: 'window' })
    for (const client of allClients) {
      if (client.id === senderId) continue
      client.postMessage(
        { crossTab: 'peer-gone', peerId: senderId } satisfies CrossTabBrokerMessage,
      )
    }
  }
}
