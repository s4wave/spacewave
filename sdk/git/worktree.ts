import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { firstWatchEmission } from '../watch.js'
import { GitRepoHandle } from './repo.js'
import { FSHandle } from '../unixfs/handle.js'
import { GitWorktreeResourceServiceClient } from './worktree_srpc.pb.js'
import type {
  GetWorktreeInfoResponse,
  StatusEntry,
  WatchStatusResponse,
} from './worktree.pb.js'

// GitWorktreeHandle represents a handle to a git worktree resource.
export class GitWorktreeHandle extends Resource {
  private service: GitWorktreeResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new GitWorktreeResourceServiceClient(resourceRef.client)
  }

  // getWorktreeInfo returns worktree metadata.
  public async getWorktreeInfo(
    signal?: AbortSignal,
  ): Promise<GetWorktreeInfoResponse> {
    return this.service.GetWorktreeInfo({}, signal)
  }

  // getRepoHandle creates a GitRepoHandle sub-resource for the linked repo.
  public async getRepoHandle(signal?: AbortSignal): Promise<GitRepoHandle> {
    const resp = await this.service.GetRepoResource({}, signal)
    if (!resp.resourceId) throw new Error('no resource ID returned for repo')
    const childRef = this.resourceRef.createRef(resp.resourceId)
    return new GitRepoHandle(childRef)
  }

  // getWorkdirHandle creates an FSHandle sub-resource for the mutable workdir.
  public async getWorkdirHandle(signal?: AbortSignal): Promise<FSHandle> {
    const resp = await this.service.GetWorkdirResource({}, signal)
    if (!resp.resourceId) {
      throw new Error('no resource ID returned for workdir')
    }
    const childRef = this.resourceRef.createRef(resp.resourceId)
    return new FSHandle(childRef)
  }

  // watchStatus streams git index state as the worktree changes.
  public watchStatus(signal?: AbortSignal): AsyncIterable<WatchStatusResponse> {
    return this.service.WatchStatus({}, signal)
  }

  // getStatusEntries returns the first status entries snapshot from WatchStatus.
  public async getStatusEntries(signal?: AbortSignal): Promise<StatusEntry[]> {
    const resp = await firstWatchEmission(this.watchStatus(signal))
    return resp?.entries ?? []
  }

  // stageFiles stages files in the git index.
  public async stageFiles(
    paths: string[],
    signal?: AbortSignal,
  ): Promise<void> {
    await this.service.StageFiles({ paths }, signal)
  }

  // unstageFiles unstages files from the git index.
  public async unstageFiles(
    paths: string[],
    signal?: AbortSignal,
  ): Promise<void> {
    await this.service.UnstageFiles({ paths }, signal)
  }
}
