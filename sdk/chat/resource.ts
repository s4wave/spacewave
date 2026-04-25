import { Engine } from '@s4wave/sdk/world/engine.js'
import { ChatChannel, ChatMessage } from './chat.pb.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'
import type {
  GetChannelInfoRequest,
  GetChannelInfoResponse,
  ListMessagesRequest,
  ListMessagesResponse,
  SendMessageRequest,
  SendMessageResponse,
  WatchMessagesRequest,
  WatchMessagesResponse,
  ChatMessageInfo,
} from './rpc/rpc.pb.js'
import type { ChatResourceService } from './rpc/rpc_srpc.pb.js'

// PRED_CHANNEL_MESSAGE is the graph predicate linking a channel to its messages.
const PRED_CHANNEL_MESSAGE = '<spacewave-chat/channel-message>'

// PRED_MESSAGE_SENDER is the graph predicate linking a message to its sender.
const PRED_MESSAGE_SENDER = '<spacewave-chat/message-sender>'

// MESSAGE_TYPE_ID is the type identifier for chat message objects.
const MESSAGE_TYPE_ID = 'spacewave-chat/message'

// ChatResource serves ChatResourceService for a single chat channel
// world object. Reads and writes via the Engine SDK.
class ChatResource implements ChatResourceService {
  private objectKey: string
  private engineRef: ClientResourceRef | undefined
  private localPeerId: string

  constructor(
    objectKey: string,
    engineRef: ClientResourceRef | undefined,
    localPeerId: string,
  ) {
    this.objectKey = objectKey
    this.engineRef = engineRef
    this.localPeerId = localPeerId
  }

  // GetChannelInfo reads the channel block and returns metadata.
  async GetChannelInfo(
    _request: GetChannelInfoRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetChannelInfoResponse> {
    if (!this.engineRef) {
      return {}
    }

    const engine = new Engine(this.engineRef)
    try {
      const channel = await this.readChannelBlock(engine, abortSignal)
      if (!channel) {
        return {}
      }
      return { name: channel.name, topic: channel.topic }
    } finally {
      engine.release()
    }
  }

  // ListMessages returns paginated messages for the channel.
  async ListMessages(
    request: ListMessagesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListMessagesResponse> {
    if (!this.engineRef) {
      return { messages: [], hasMore: false }
    }

    const engine = new Engine(this.engineRef)
    try {
      const limit = request.limit && request.limit > 0 ? request.limit : 50
      const allMessages = await this.readMessages(
        engine,
        limit + 1,
        abortSignal,
      )

      // Apply beforeKey filter if specified.
      let filtered = allMessages
      if (request.beforeKey) {
        filtered = allMessages.filter(
          (m) => (m.objectKey ?? '') < request.beforeKey!,
        )
      }

      // Sort chronologically (oldest first).
      filtered.sort((a, b) =>
        (a.objectKey ?? '').localeCompare(b.objectKey ?? ''),
      )

      const hasMore = filtered.length > limit
      if (hasMore) {
        filtered = filtered.slice(filtered.length - limit)
      }

      return { messages: filtered, hasMore }
    } finally {
      engine.release()
    }
  }

  // WatchMessages streams messages, re-reading on every world change.
  async *WatchMessages(
    _request: WatchMessagesRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<WatchMessagesResponse> {
    if (!this.engineRef) {
      return
    }

    const engine = new Engine(this.engineRef)
    try {
      let lastSeqno = 0n
      const knownKeys = new Set<string>()
      for (;;) {
        if (abortSignal?.aborted) return

        const messages = await this.readMessages(
          engine,
          1000,
          abortSignal,
          knownKeys,
        )

        if (messages.length > 0) {
          yield { messages }
          for (const m of messages) {
            knownKeys.add(m.objectKey ?? '')
          }
        }

        // Get the seqno of the state we just read.
        const resp = await engine.getSeqno(abortSignal)
        lastSeqno = resp.seqno ?? 0n

        // Block until the world state advances past the snapshot we read.
        await engine.waitSeqno(lastSeqno + 1n, abortSignal)
      }
    } catch (err) {
      if (abortSignal?.aborted) return
      throw err
    } finally {
      engine.release()
    }
  }

  // SendMessage creates a new message object linked to this channel.
  async SendMessage(
    request: SendMessageRequest,
    abortSignal?: AbortSignal,
  ): Promise<SendMessageResponse> {
    if (!this.engineRef) {
      throw new Error('no engine available')
    }

    const engine = new Engine(this.engineRef)
    try {
      const now = Date.now()
      const rand = String(Math.random()).slice(2, 8)
      const msgKey = `chat/message/${now}${rand}`

      const tx = await engine.newTransaction(true, abortSignal)
      try {
        // Create message object with empty root ref.
        const objState = await tx.createObject(msgKey, {}, abortSignal)
        try {
          const cursor = await objState.accessWorldState(undefined, abortSignal)
          try {
            const msg = ChatMessage.create({
              senderPeerId: this.localPeerId,
              content: { content: { case: 'text', value: request.text ?? '' } },
              createdAt: new Date(now),
              replyToKey: request.replyToKey ?? '',
            })
            const msgData = ChatMessage.toBinary(msg)
            await cursor.putBlock({ data: msgData }, abortSignal)
          } finally {
            cursor.release()
          }
        } finally {
          objState.release()
        }

        // Set message object type.
        await setObjectType(tx, msgKey, MESSAGE_TYPE_ID, abortSignal)

        // Link channel to message.
        await tx.setGraphQuad(
          keyToIRI(this.objectKey),
          PRED_CHANNEL_MESSAGE,
          keyToIRI(msgKey),
          undefined,
          abortSignal,
        )

        // Link message to sender peer.
        await tx.setGraphQuad(
          keyToIRI(msgKey),
          PRED_MESSAGE_SENDER,
          keyToIRI('peer/' + this.localPeerId),
          undefined,
          abortSignal,
        )

        await tx.commit(abortSignal)
        return { messageKey: msgKey }
      } finally {
        tx.release()
      }
    } finally {
      engine.release()
    }
  }

  // readChannelBlock reads the ChatChannel block from the world.
  private async readChannelBlock(
    engine: Engine,
    abortSignal?: AbortSignal,
  ): Promise<ChatChannel | null> {
    const tx = await engine.newTransaction(false, abortSignal)
    try {
      const objectState = await tx.getObject(this.objectKey, abortSignal)
      if (!objectState) return null
      try {
        const cursor = await objectState.accessWorldState(
          undefined,
          abortSignal,
        )
        try {
          const blockResp = await cursor.getBlock({}, abortSignal)
          if (!blockResp.found || !blockResp.data) return null
          return ChatChannel.fromBinary(blockResp.data)
        } finally {
          cursor.release()
        }
      } finally {
        objectState.release()
      }
    } finally {
      tx.release()
    }
  }

  // readMessages reads message objects linked to this channel via graph quads.
  // When skipKeys is provided, messages with keys in the set are not re-read.
  private async readMessages(
    engine: Engine,
    limit: number,
    abortSignal?: AbortSignal,
    skipKeys?: Set<string>,
  ): Promise<ChatMessageInfo[]> {
    const tx = await engine.newTransaction(false, abortSignal)
    try {
      // Look up message keys linked to this channel.
      const quadResult = await tx.lookupGraphQuads(
        keyToIRI(this.objectKey),
        PRED_CHANNEL_MESSAGE,
        undefined,
        undefined,
        limit,
        abortSignal,
      )
      const quads = quadResult.quads ?? []

      // Extract and sort message keys, skipping already-known keys.
      const msgKeys = quads
        .filter((q) => !!q.obj)
        .map((q) => iriToKey(q.obj!))
        .filter((key) => !skipKeys || !skipKeys.has(key))
        .sort()

      // Read each message block.
      const messages: ChatMessageInfo[] = []
      for (const key of msgKeys) {
        const objectState = await tx.getObject(key, abortSignal)
        if (!objectState) continue
        try {
          const cursor = await objectState.accessWorldState(
            undefined,
            abortSignal,
          )
          try {
            const blockResp = await cursor.getBlock({}, abortSignal)
            if (!blockResp.found || !blockResp.data) continue
            const msg = ChatMessage.fromBinary(blockResp.data)
            let text = ''
            if (msg.content?.content?.case === 'text') {
              text = msg.content.content.value
            }
            messages.push({
              objectKey: key,
              senderPeerId: msg.senderPeerId ?? '',
              text,
              createdAt: msg.createdAt,
              replyToKey: msg.replyToKey ?? '',
            })
          } finally {
            cursor.release()
          }
        } finally {
          objectState.release()
        }
      }

      return messages
    } finally {
      tx.release()
    }
  }

  // dispose releases the engine ref if still held.
  dispose(): void {
    this.engineRef?.release()
    this.engineRef = undefined
  }
}

export { ChatResource }
