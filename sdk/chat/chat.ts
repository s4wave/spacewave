import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { ChatResourceServiceClient } from './rpc/rpc_srpc.pb.js'
import type {
  ChatMessageInfo,
  GetChannelInfoResponse,
  ListMessagesResponse,
  SendMessageResponse,
  WatchChannelInfoResponse,
  WatchMessagesResponse,
} from './rpc/rpc.pb.js'

// ChatChannelTypeID is the type identifier for chat channel objects.
export const ChatChannelTypeID = 'spacewave-chat/channel'

// IChatHandle contains the ChatHandle interface.
export interface IChatHandle {
  // watchMessages streams new messages in real-time.
  watchMessages(abortSignal?: AbortSignal): AsyncIterable<ChatMessageInfo[]>

  // sendMessage sends a message to the channel.
  sendMessage(
    text: string,
    replyToKey?: string,
    abortSignal?: AbortSignal,
  ): Promise<SendMessageResponse>

  // watchChannelInfo streams channel metadata, then again after each change.
  watchChannelInfo(
    abortSignal?: AbortSignal,
  ): AsyncIterable<GetChannelInfoResponse>

  // listMessages returns paginated messages.
  listMessages(
    beforeKey?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ): Promise<ListMessagesResponse>

  // release releases the resource.
  release(): void

  // Symbol.dispose releases the resource for use with a 'using' statement.
  [Symbol.dispose](): void
}

// ChatHandle represents a handle to a chat channel resource.
// Each instance maps 1:1 to a Go ChatResource on the backend.
export class ChatHandle extends Resource implements IChatHandle {
  private service: ChatResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new ChatResourceServiceClient(resourceRef.client)
  }

  // watchMessages streams new messages in real-time.
  public async *watchMessages(
    abortSignal?: AbortSignal,
  ): AsyncIterable<ChatMessageInfo[]> {
    const stream = this.service.WatchMessages({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchMessagesResponse>) {
      yield resp.messages ?? []
    }
  }

  // sendMessage sends a message to the channel.
  public async sendMessage(
    text: string,
    replyToKey?: string,
    abortSignal?: AbortSignal,
  ): Promise<SendMessageResponse> {
    return await this.service.SendMessage(
      { text, replyToKey: replyToKey ?? '' },
      abortSignal,
    )
  }

  // watchChannelInfo streams channel metadata, then again after each change.
  public async *watchChannelInfo(
    abortSignal?: AbortSignal,
  ): AsyncIterable<GetChannelInfoResponse> {
    const stream = this.service.WatchChannelInfo({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchChannelInfoResponse>) {
      yield resp.info ?? {}
    }
  }

  // listMessages returns paginated messages.
  public async listMessages(
    beforeKey?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ): Promise<ListMessagesResponse> {
    return await this.service.ListMessages(
      { beforeKey: beforeKey ?? '', limit: limit ?? 0 },
      abortSignal,
    )
  }
}
