/**
 * v86fs-bridge.ts - V86FSAdapter backed by v86fs SRPC client.
 *
 * Bridges the v86 VirtioV86FS adapter callback interface to a remote
 * v86fs SRPC server over a bidirectional stream. Each adapter callback
 * sends a tagged request and resolves the reply callback when the
 * matching tagged response arrives.
 */

import { pushable } from 'it-pushable'
import type { V86fsMessage as V86fsMessageType } from '@go/github.com/aperturerobotics/hydra/unixfs/v86fs/v86fs.pb.js'
import { V86fsMessage } from '@go/github.com/aperturerobotics/hydra/unixfs/v86fs/v86fs.pb.js'
import { V86fsServiceClient } from '@go/github.com/aperturerobotics/hydra/unixfs/v86fs/v86fs_srpc.pb.js'
import type { ProtoRpc } from 'starpc'

// Reply handler receives the full V86fsMessage and extracts what it needs.
type ReplyHandler = (msg: V86fsMessageType) => void

/**
 * Creates a V86FSAdapter backed by a v86fs SRPC client.
 *
 * The adapter implements all v86fs callbacks by forwarding them as
 * tagged SRPC requests and dispatching replies back to the caller.
 *
 * @param rpc - SRPC ProtoRpc connection to the v86fs server.
 * @returns adapter compatible with V86 constructor's virtio_v86fs_adapter option.
 */
export function createV86fsSrpcAdapter(rpc: ProtoRpc): {
  adapter: any
  close: () => void
} {
  const client = new V86fsServiceClient(rpc)
  const outgoing = pushable<V86fsMessageType>({ objectMode: true })
  const pending = new Map<number, ReplyHandler>()
  let nextTag = 1

  // Open bidirectional SRPC stream.
  const responses = client.RelayV86fs(outgoing)

  // Read responses in background, dispatch by tag.
  const readerDone = (async () => {
    for await (const msg of responses) {
      const tag = msg.tag ?? 0
      if (tag === 0) {
        // Notifications (invalidate, mount, umount) have no tag.
        // Handled by the v86fs virtio device's page cache internally.
        continue
      }
      const handler = pending.get(tag)
      if (handler) {
        pending.delete(tag)
        handler(msg)
      }
    }
  })().catch(() => {
    // Stream closed. Reject all pending requests.
    for (const [, handler] of pending) {
      handler({ tag: 0, body: { case: 'errorReply', value: { status: 5 } } })
    }
    pending.clear()
  })

  function send(
    body: V86fsMessageType['body'],
    handler: ReplyHandler,
  ): void {
    const tag = nextTag++
    pending.set(tag, handler)
    outgoing.push({ tag, body })
  }

  const adapter = {
    onMount(
      name: string,
      reply: (status: number, root_inode_id: number, mode: number) => void,
    ): void {
      send({ case: 'mountRequest', value: { name } }, (msg) => {
        if (msg.body?.case === 'errorReply') {
          reply(msg.body.value.status ?? 5, 0, 0)
          return
        }
        const r = msg.body?.case === 'mountReply' ? msg.body.value : undefined
        reply(r?.status ?? 5, Number(r?.rootInodeId ?? 0n), r?.mode ?? 0)
      })
    },

    onLookup(
      parent_id: number,
      name: string,
      reply: (
        status: number,
        inode_id: number,
        mode: number,
        size: number,
      ) => void,
    ): void {
      send(
        {
          case: 'lookupRequest',
          value: { parentId: BigInt(parent_id), name },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0, 0, 0)
            return
          }
          const r =
            msg.body?.case === 'lookupReply' ? msg.body.value : undefined
          reply(
            r?.status ?? 5,
            Number(r?.inodeId ?? 0n),
            r?.mode ?? 0,
            Number(r?.size ?? 0n),
          )
        },
      )
    },

    onGetattr(
      inode_id: number,
      reply: (
        status: number,
        mode: number,
        size: number,
        mtime_sec: number,
        mtime_nsec: number,
      ) => void,
    ): void {
      send(
        { case: 'getattrRequest', value: { inodeId: BigInt(inode_id) } },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0, 0, 0, 0)
            return
          }
          const r =
            msg.body?.case === 'getattrReply' ? msg.body.value : undefined
          reply(
            r?.status ?? 5,
            r?.mode ?? 0,
            Number(r?.size ?? 0n),
            Number(r?.mtimeSec ?? 0n),
            r?.mtimeNsec ?? 0,
          )
        },
      )
    },

    onReaddir(
      dir_id: number,
      reply: (
        status: number,
        entries: Array<{ inode_id: number; dt_type: number; name: string }>,
      ) => void,
    ): void {
      send(
        { case: 'readdirRequest', value: { dirId: BigInt(dir_id) } },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, [])
            return
          }
          const r =
            msg.body?.case === 'readdirReply' ? msg.body.value : undefined
          const entries = (r?.entries ?? []).map((e) => ({
            inode_id: Number(e.inodeId ?? 0n),
            dt_type: e.dtType ?? 0,
            name: e.name ?? '',
          }))
          reply(r?.status ?? 5, entries)
        },
      )
    },

    onOpen(
      inode_id: number,
      flags: number,
      reply: (status: number, handle_id: number) => void,
    ): void {
      send(
        {
          case: 'openRequest',
          value: { inodeId: BigInt(inode_id), flags },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0)
            return
          }
          const r =
            msg.body?.case === 'openReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, Number(r?.handleId ?? 0n))
        },
      )
    },

    onClose(
      handle_id: number,
      reply: (status: number) => void,
    ): void {
      send(
        { case: 'closeRequest', value: { handleId: BigInt(handle_id) } },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5)
            return
          }
          const r =
            msg.body?.case === 'closeReply' ? msg.body.value : undefined
          reply(r?.status ?? 5)
        },
      )
    },

    onRead(
      handle_id: number,
      offset: number,
      size: number,
      reply: (status: number, data: Uint8Array) => void,
    ): void {
      send(
        {
          case: 'readRequest',
          value: {
            handleId: BigInt(handle_id),
            offset: BigInt(offset),
            size,
          },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, new Uint8Array(0))
            return
          }
          const r =
            msg.body?.case === 'readReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, r?.data ?? new Uint8Array(0))
        },
      )
    },

    onCreate(
      parent_id: number,
      name: string,
      mode: number,
      reply: (status: number, inode_id: number, mode: number) => void,
    ): void {
      send(
        {
          case: 'createRequest',
          value: { parentId: BigInt(parent_id), name, mode },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0, 0)
            return
          }
          const r =
            msg.body?.case === 'createReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, Number(r?.inodeId ?? 0n), r?.mode ?? 0)
        },
      )
    },

    onWrite(
      inode_id: number,
      offset: number,
      data: Uint8Array,
      reply: (status: number, bytes_written: number) => void,
    ): void {
      send(
        {
          case: 'writeRequest',
          value: { inodeId: BigInt(inode_id), offset: BigInt(offset), data },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0)
            return
          }
          const r =
            msg.body?.case === 'writeReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, r?.bytesWritten ?? 0)
        },
      )
    },

    onMkdir(
      parent_id: number,
      name: string,
      mode: number,
      reply: (status: number, inode_id: number, mode: number) => void,
    ): void {
      send(
        {
          case: 'mkdirRequest',
          value: { parentId: BigInt(parent_id), name, mode },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0, 0)
            return
          }
          const r =
            msg.body?.case === 'mkdirReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, Number(r?.inodeId ?? 0n), r?.mode ?? 0)
        },
      )
    },

    onSetattr(
      inode_id: number,
      valid: number,
      mode: number,
      size: number,
      reply: (status: number) => void,
    ): void {
      send(
        {
          case: 'setattrRequest',
          value: {
            inodeId: BigInt(inode_id),
            valid,
            mode,
            size: BigInt(size),
          },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5)
            return
          }
          const r =
            msg.body?.case === 'setattrReply' ? msg.body.value : undefined
          reply(r?.status ?? 5)
        },
      )
    },

    onFsync(inode_id: number, reply: (status: number) => void): void {
      send(
        { case: 'fsyncRequest', value: { inodeId: BigInt(inode_id) } },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5)
            return
          }
          const r =
            msg.body?.case === 'fsyncReply' ? msg.body.value : undefined
          reply(r?.status ?? 5)
        },
      )
    },

    onUnlink(
      parent_id: number,
      name: string,
      reply: (status: number) => void,
    ): void {
      send(
        {
          case: 'unlinkRequest',
          value: { parentId: BigInt(parent_id), name },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5)
            return
          }
          const r =
            msg.body?.case === 'unlinkReply' ? msg.body.value : undefined
          reply(r?.status ?? 5)
        },
      )
    },

    onRename(
      old_parent_id: number,
      old_name: string,
      new_parent_id: number,
      new_name: string,
      reply: (status: number) => void,
    ): void {
      send(
        {
          case: 'renameRequest',
          value: {
            oldParentId: BigInt(old_parent_id),
            oldName: old_name,
            newParentId: BigInt(new_parent_id),
            newName: new_name,
          },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5)
            return
          }
          const r =
            msg.body?.case === 'renameReply' ? msg.body.value : undefined
          reply(r?.status ?? 5)
        },
      )
    },

    onSymlink(
      parent_id: number,
      name: string,
      target: string,
      reply: (status: number, inode_id: number, mode: number) => void,
    ): void {
      send(
        {
          case: 'symlinkRequest',
          value: { parentId: BigInt(parent_id), name, target },
        },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, 0, 0)
            return
          }
          const r =
            msg.body?.case === 'symlinkReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, Number(r?.inodeId ?? 0n), r?.mode ?? 0)
        },
      )
    },

    onReadlink(
      inode_id: number,
      reply: (status: number, target: string) => void,
    ): void {
      send(
        { case: 'readlinkRequest', value: { inodeId: BigInt(inode_id) } },
        (msg) => {
          if (msg.body?.case === 'errorReply') {
            reply(msg.body.value.status ?? 5, '')
            return
          }
          const r =
            msg.body?.case === 'readlinkReply' ? msg.body.value : undefined
          reply(r?.status ?? 5, r?.target ?? '')
        },
      )
    },

    onStatfs(
      reply: (
        status: number,
        blocks: number,
        bfree: number,
        bavail: number,
        files: number,
        ffree: number,
        bsize: number,
      ) => void,
    ): void {
      send({ case: 'statfsRequest', value: {} }, (msg) => {
        if (msg.body?.case === 'errorReply') {
          reply(msg.body.value.status ?? 5, 0, 0, 0, 0, 0, 0)
          return
        }
        const r =
          msg.body?.case === 'statfsReply' ? msg.body.value : undefined
        reply(
          r?.status ?? 5,
          Number(r?.blocks ?? 0n),
          Number(r?.bfree ?? 0n),
          Number(r?.bavail ?? 0n),
          Number(r?.files ?? 0n),
          Number(r?.ffree ?? 0n),
          r?.bsize ?? 4096,
        )
      })
    },
  }

  return {
    adapter,
    close() {
      outgoing.end()
    },
  }
}
