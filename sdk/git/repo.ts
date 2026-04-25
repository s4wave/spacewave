import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { GitRepoResourceServiceClient } from './repo_srpc.pb.js'
import { FSHandle } from '../unixfs/handle.js'
import type {
  CommitInfo,
  GetDiffStatResponse,
  GetRepoInfoResponse,
  ListRefsResponse,
  LogResponse,
} from './repo.pb.js'

// GitRepoHandle represents a handle to a git repository resource.
export class GitRepoHandle extends Resource {
  private service: GitRepoResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new GitRepoResourceServiceClient(resourceRef.client)
  }

  // listRefs lists all branches and tags in the repository.
  public async listRefs(signal?: AbortSignal): Promise<ListRefsResponse> {
    return this.service.ListRefs({}, signal)
  }

  // resolveRef resolves a ref name to a commit hash and tree hash.
  public async resolveRef(
    refName: string,
    signal?: AbortSignal,
  ): Promise<{ commitHash: string; treeHash: string }> {
    const resp = await this.service.ResolveRef({ refName }, signal)
    return {
      commitHash: resp.commitHash ?? '',
      treeHash: resp.treeHash ?? '',
    }
  }

  // getRepoInfo returns repository overview information.
  public async getRepoInfo(signal?: AbortSignal): Promise<GetRepoInfoResponse> {
    return this.service.GetRepoInfo({}, signal)
  }

  // getTreeResource creates an FSHandle sub-resource for a ref's tree.
  public async getTreeResource(
    refName?: string,
    signal?: AbortSignal,
  ): Promise<FSHandle> {
    const resp = await this.service.GetTreeResource(
      { refName: refName ?? '' },
      signal,
    )
    if (!resp.resourceId) throw new Error('no resource ID returned for tree')
    const childRef = this.resourceRef.createRef(resp.resourceId)
    return new FSHandle(childRef)
  }

  // getRepoFilesystemHandle creates an FSHandle sub-resource for the repository filesystem.
  public async getRepoFilesystemHandle(
    writable?: boolean,
    signal?: AbortSignal,
  ): Promise<FSHandle> {
    const resp = await this.service.GetRepoFilesystemResource(
      { writable: writable ?? false },
      signal,
    )
    if (!resp.resourceId) {
      throw new Error('no resource ID returned for repository filesystem')
    }
    const childRef = this.resourceRef.createRef(resp.resourceId)
    return new FSHandle(childRef)
  }

  // log returns a paginated list of commits starting from a ref.
  public async log(
    refName?: string,
    offset?: number,
    limit?: number,
    signal?: AbortSignal,
  ): Promise<LogResponse> {
    return this.service.Log(
      { refName: refName ?? '', offset: offset ?? 0, limit: limit ?? 50 },
      signal,
    )
  }

  // getCommit returns full metadata for a single commit.
  public async getCommit(
    hash: string,
    signal?: AbortSignal,
  ): Promise<CommitInfo | undefined> {
    const resp = await this.service.GetCommit({ hash }, signal)
    return resp.commit
  }

  // getDiffStat returns diff stats between two refs.
  public async getDiffStat(
    refA: string,
    refB?: string,
    signal?: AbortSignal,
  ): Promise<GetDiffStatResponse> {
    return this.service.GetDiffStat({ refA, refB: refB ?? '' }, signal)
  }

  // resolveRefToCommit resolves a ref name and fetches the full commit info.
  public async resolveRefToCommit(
    refName: string,
    signal?: AbortSignal,
  ): Promise<CommitInfo | undefined> {
    const resolved = await this.resolveRef(refName, signal)
    if (!resolved.commitHash) return undefined
    return this.getCommit(resolved.commitHash, signal)
  }

  // paginatedLog returns an async iterable that yields pages of commits.
  public async *paginatedLog(
    refName?: string,
    signal?: AbortSignal,
  ): AsyncIterable<CommitInfo[]> {
    let offset = 0
    const pageSize = 50
    for (;;) {
      const resp = await this.log(refName, offset, pageSize, signal)
      const commits = resp.commits ?? []
      if (commits.length === 0) return
      yield commits
      if (!resp.hasMore) return
      offset += commits.length
    }
  }
}
