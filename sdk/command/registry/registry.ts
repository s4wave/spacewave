import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { Command } from '../command.pb.js'
import {
  CommandRegistryResourceService,
  CommandRegistryResourceServiceClient,
} from './registry_srpc.pb.js'
import {
  GetSubItemsResponse,
  InvokeCommandResponse,
  RegisterCommandResponse,
  SetActiveResponse,
  SetEnabledResponse,
  WatchCommandsResponse,
} from './registry.pb.js'

// CommandsManager is a resource that provides command registration and invocation.
// Plugins register commands via registerCommand and watch for changes via watchCommands.
export class CommandsManager extends Resource {
  // service is the command registry resource service.
  private service: CommandRegistryResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new CommandRegistryResourceServiceClient(resourceRef.client)
  }

  // registerCommand registers a command with an optional handler resource.
  public async registerCommand(
    command: Command,
    handlerResourceId?: number,
    abortSignal?: AbortSignal,
  ): Promise<RegisterCommandResponse> {
    return await this.service.RegisterCommand(
      { command, handlerResourceId },
      abortSignal,
    )
  }

  // setActive sets the active state of a registration.
  public async setActive(
    resourceId: number,
    active: boolean,
    abortSignal?: AbortSignal,
  ): Promise<SetActiveResponse> {
    return await this.service.SetActive({ resourceId, active }, abortSignal)
  }

  // setEnabled sets the enabled state of a registration.
  public async setEnabled(
    resourceId: number,
    enabled: boolean,
    abortSignal?: AbortSignal,
  ): Promise<SetEnabledResponse> {
    return await this.service.SetEnabled({ resourceId, enabled }, abortSignal)
  }

  // watchCommands streams the full command registry with active state.
  public watchCommands(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchCommandsResponse> {
    return this.service.WatchCommands({}, abortSignal)
  }

  // getSubItems queries a command's sub-items from the active registration.
  public async getSubItems(
    commandId: string,
    query: string,
    abortSignal?: AbortSignal,
  ): Promise<GetSubItemsResponse> {
    return await this.service.GetSubItems({ commandId, query }, abortSignal)
  }

  // invokeCommand invokes a registered command with optional arguments.
  public async invokeCommand(
    commandId: string,
    args?: Record<string, string>,
    abortSignal?: AbortSignal,
  ): Promise<InvokeCommandResponse> {
    return await this.service.InvokeCommand({ commandId, args }, abortSignal)
  }
}
