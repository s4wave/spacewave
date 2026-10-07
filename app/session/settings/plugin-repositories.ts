import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import {
  GitWorktreeHandle,
  GitWorktreeTypeID,
} from '@s4wave/sdk/git/worktree.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// The Space resource stores each GitHub repository at plugin-repos/<owner>/<repo>
// with its worktree beneath it.
const repositoryKeyPrefix = 'plugin-repos/'
const worktreeKeySuffix = '/worktree'

// remoteHeadRef is the remote-tracking ref a single-branch fetch moves.
const remoteHeadRef = 'refs/remotes/origin/HEAD'

/** PluginRepository is a GitHub repository held in the developer Space. */
export interface PluginRepository {
  // name is the lowercase owner/repo.
  name: string
  // pinnedCommit is the commit the worktree has checked out.
  pinnedCommit: string
  // fetchedCommit is the newest commit fetched from GitHub.
  fetchedCommit: string
}

/** PluginRepositoryInventory is what the plugin settings screen shows. */
export interface PluginRepositoryInventory {
  // devices lists the Space's Device object keys that can run the fetch.
  devices: string[]
  repositories: PluginRepository[]
}

/**
 * readPluginRepositories reads the developer Space's Devices and repositories.
 * Run it as a World query so each fetch's World revision re-reads the commits.
 */
export async function readPluginRepositories(
  world: IWorldState,
  signal?: AbortSignal,
): Promise<PluginRepositoryInventory> {
  const [devices, worktrees] = await Promise.all([
    world.listObjectsWithType(DeviceTypeID, signal),
    world.listObjectsWithType(GitWorktreeTypeID, signal),
  ])
  const repositories = await Promise.all(
    worktrees
      .filter(
        (key) =>
          key.startsWith(repositoryKeyPrefix) &&
          key.endsWith(worktreeKeySuffix),
      )
      .sort()
      .map((key) => readPluginRepository(world, key, signal)),
  )
  return { devices, repositories }
}

/** readPluginRepository reads the pinned and fetched commits of one worktree. */
async function readPluginRepository(
  world: IWorldState,
  worktreeKey: string,
  signal?: AbortSignal,
): Promise<PluginRepository> {
  // Open the worktree and its repository for this read only.
  const access = await world.accessTypedObject(worktreeKey, signal)
  using worktree = new GitWorktreeHandle(
    world.getResourceRef().createRef(access.resourceId ?? 0),
  )
  const [info, repoHandle] = await Promise.all([
    worktree.getWorktreeInfo(signal),
    worktree.getRepoHandle(signal),
  ])
  using repo = repoHandle
  const fetched = await repo.resolveRef(remoteHeadRef, signal)
  return {
    name: worktreeKey.slice(
      repositoryKeyPrefix.length,
      -worktreeKeySuffix.length,
    ),
    pinnedCommit: info.headCommitHash ?? '',
    fetchedCommit: fetched.commitHash,
  }
}
