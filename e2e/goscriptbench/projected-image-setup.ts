import { mountSpace } from "@s4wave/app/space/space.js";
import { UNIXFS_OBJECT_KEY } from "@s4wave/core/space/world/ops/init-unixfs.js";
import { FSHandle } from "@s4wave/sdk/unixfs/index.js";

type ResourceLike = { release?: () => void };

interface ProjectedImageSetupArgs {
  action: "upload" | "verify";
  sessionIndex: number;
  spaceId: string;
  path: string;
  encodedBytes: number;
  sha256: string;
  fixtureBase64: string;
  deadlineMs: number;
}

interface FixtureProof {
  action: "upload" | "verify";
  path: string;
  encodedBytes: number;
  sha256: string;
}

function cleanupTracker() {
  const stack: ResourceLike[] = [];
  const cleanup = <T extends ResourceLike | undefined | null>(
    resource: T,
  ): T => {
    if (resource?.release) stack.push(resource);
    return resource;
  };
  return {
    cleanup,
    releaseAll() {
      for (;;) {
        const resource = stack.pop();
        if (!resource) return;
        resource.release?.();
      }
    },
  };
}

function decodeBase64(data: string): Uint8Array {
  const raw = atob(data);
  const bytes = new Uint8Array(raw.length);
  for (let idx = 0; idx < raw.length; idx++) {
    bytes[idx] = raw.charCodeAt(idx);
  }
  return bytes;
}

async function sha256Hex(data: Uint8Array): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", data));
  return Array.from(digest, (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
}

async function readFixture(
  rootHandle: FSHandle,
  args: ProjectedImageSetupArgs,
  signal: AbortSignal,
): Promise<FixtureProof> {
  const file = await rootHandle.lookupPath(args.path, signal);
  try {
    const read = await file.handle.readAt(
      0n,
      BigInt(args.encodedBytes),
      signal,
    );
    if (
      read.bytesRead !== BigInt(args.encodedBytes) ||
      read.data.byteLength !== args.encodedBytes
    ) {
      throw new Error(
        `fixture read ${read.bytesRead}/${read.data.byteLength} bytes, expected ${args.encodedBytes}`,
      );
    }
    const sha256 = await sha256Hex(read.data);
    if (sha256 !== args.sha256) {
      throw new Error(`fixture SHA-256 ${sha256}, expected ${args.sha256}`);
    }
    return {
      action: args.action,
      path: args.path,
      encodedBytes: read.data.byteLength,
      sha256,
    };
  } finally {
    file.handle.release();
  }
}

async function withRootHandle<T>(
  args: ProjectedImageSetupArgs,
  signal: AbortSignal,
  fn: (rootHandle: FSHandle) => Promise<T>,
): Promise<T> {
  const root = globalThis.__s4wave_debug?.root;
  if (!root) throw new Error("missing __s4wave_debug.root");

  const tracker = cleanupTracker();
  try {
    const mounted = await root.mountSessionByIdx(
      { sessionIdx: args.sessionIndex },
      signal,
    );
    const session = tracker.cleanup(mounted?.session);
    if (!session) throw new Error("mountSessionByIdx returned no session");

    const space = tracker.cleanup(
      await mountSpace({
        session,
        spaceResp: {
          sharedObjectRef: {
            providerResourceRef: { id: args.spaceId },
          },
        },
        abortSignal: signal,
        cleanup: tracker.cleanup,
      }),
    );
    const world = tracker.cleanup(
      await space.accessWorldState(args.action === "upload", signal),
    );
    const access = await world.accessTypedObject(UNIXFS_OBJECT_KEY, signal);
    if (!access.resourceId) {
      throw new Error("accessTypedObject returned no UnixFS resource ID");
    }
    const rootHandle = tracker.cleanup(
      new FSHandle(world.getResourceRef().createRef(access.resourceId)),
    );
    return await fn(rootHandle);
  } finally {
    tracker.releaseAll();
  }
}

export default async function projectedImageSetup(
  args: ProjectedImageSetupArgs,
): Promise<FixtureProof> {
  const signal = AbortSignal.timeout(args.deadlineMs);
  return await withRootHandle(args, signal, async (rootHandle) => {
    if (args.action === "upload") {
      const bytes = decodeBase64(args.fixtureBase64);
      if (bytes.byteLength !== args.encodedBytes) {
        throw new Error(
          `fixture payload has ${bytes.byteLength} bytes, expected ${args.encodedBytes}`,
        );
      }
      const written = await rootHandle.uploadFile(
        args.path,
        BigInt(args.encodedBytes),
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(bytes);
            controller.close();
          },
        }),
        0o644,
        undefined,
        signal,
      );
      if (written !== BigInt(args.encodedBytes)) {
        throw new Error(
          `fixture upload wrote ${written} bytes, expected ${args.encodedBytes}`,
        );
      }
    }
    return await readFixture(rootHandle, args, signal);
  });
}
