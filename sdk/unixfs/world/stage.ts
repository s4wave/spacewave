import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { WorldStage } from '../../world/stage.js'
import type { HandleUploadTreeRequest } from '../handle.pb.js'
import { UnixFSStageResourceServiceClient } from './stage_srpc.pb.js'

// StagedTreeFile is one file of a tree built in a World stage. path is
// slash-separated and relative to the tree root; parent directories are
// created as needed.
export interface StagedTreeFile {
  path: string
  data: Uint8Array
  mode?: number
}

// stagedTreeFrameMaxBytes bounds one data frame of the entry stream.
const stagedTreeFrameMaxBytes = 64 * 1024

// buildStagedTree builds a new UnixFS tree of files in stage and returns its
// root, the root of a unixfs/fs-node typed object. The stage holds the tree:
// hold it until the World transaction that adopts the root returns. Once the
// stage is released, blocks no committed object names can be collected.
export async function buildStagedTree(
  stage: WorldStage,
  files: StagedTreeFile[],
  abortSignal?: AbortSignal,
): Promise<ObjectRef> {
  async function* entries(): AsyncIterable<HandleUploadTreeRequest> {
    for (const file of files) {
      yield {
        body: {
          case: 'fileStart',
          value: {
            path: file.path,
            totalSize: BigInt(file.data.byteLength),
            mode: file.mode ?? 0o644,
          },
        },
      }
      for (let i = 0; i < file.data.byteLength; i += stagedTreeFrameMaxBytes) {
        yield {
          body: {
            case: 'data',
            value: file.data.subarray(i, i + stagedTreeFrameMaxBytes),
          },
        }
      }
    }
  }

  const service = new UnixFSStageResourceServiceClient(stage.resourceRef.client)
  const response = await service.BuildTree(entries(), abortSignal)
  if (!response.rootRef) {
    throw new Error('staged tree response has no root')
  }
  return response.rootRef
}
