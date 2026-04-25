import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { WizardResourceServiceClient } from './wizard_srpc.pb.js'
import type {
  GitCloneProgress,
  StartGitCloneResponse,
  WizardState,
  WatchGitCloneProgressResponse,
  WatchWizardStateResponse,
  UpdateWizardStateResponse,
} from './wizard.pb.js'

// WizardHandle represents a handle to a wizard resource.
export class WizardHandle extends Resource {
  private service: WizardResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new WizardResourceServiceClient(resourceRef.client)
  }

  public async *watchState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WizardState> {
    const stream = this.service.WatchWizardState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchWizardStateResponse>) {
      if (resp.state) {
        yield resp.state
      }
    }
  }

  public async updateState(
    opts: {
      step?: number
      name?: string
      configData?: Uint8Array
    },
    abortSignal?: AbortSignal,
  ): Promise<UpdateWizardStateResponse> {
    return this.service.UpdateWizardState(
      {
        step: opts.step ?? -1,
        name: opts.name ?? '',
        configData: opts.configData,
        hasConfigData: opts.configData !== undefined,
      },
      abortSignal,
    )
  }

  public async startGitClone(
    opts: {
      objectKey: string
      name: string
      configData: Uint8Array
      opSender?: string
    },
    abortSignal?: AbortSignal,
  ): Promise<StartGitCloneResponse> {
    return this.service.StartGitClone(
      {
        objectKey: opts.objectKey,
        name: opts.name,
        configData: opts.configData,
        opSender: opts.opSender ?? '',
      },
      abortSignal,
    )
  }

  public async *watchGitCloneProgress(
    abortSignal?: AbortSignal,
  ): AsyncIterable<GitCloneProgress> {
    const stream = this.service.WatchGitCloneProgress({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchGitCloneProgressResponse>) {
      if (resp.progress) {
        yield resp.progress
      }
    }
  }
}
