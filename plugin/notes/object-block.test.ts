import { describe, expect, it, vi } from "vitest";

import { createObjectWithBlockData } from "./object-block.js";

describe("object block helpers", () => {
  it("creates a new object from the written world-storage root", async () => {
    const release = vi.fn();
    const cursor = {
      putBlock: vi.fn(() =>
        Promise.resolve({
          ref: {
            hash: {
              hashType: 1,
              hash: new Uint8Array([1, 2, 3]),
            },
          },
        }),
      ),
      getRef: vi.fn(() =>
        Promise.resolve({
          ref: {
            bucketId: "bucket-1",
          },
        }),
      ),
      release,
      [Symbol.dispose]: release,
    };
    const createObject = vi.fn(() =>
      Promise.resolve({
        release,
        [Symbol.dispose]: release,
      }),
    );
    const stage = {
      buildStorageCursor: vi.fn(() => Promise.resolve(cursor)),
      [Symbol.dispose]: vi.fn(),
    };
    const worldState = {
      stageWorldState: vi.fn(() => Promise.resolve(stage)),
      createObject,
    };

    await createObjectWithBlockData(
      worldState as never,
      "blog/site",
      new Uint8Array([4, 5, 6]),
    );

    expect(worldState.stageWorldState).toHaveBeenCalledWith(undefined);
    expect(stage.buildStorageCursor).toHaveBeenCalledWith(undefined);
    expect(cursor.putBlock).toHaveBeenCalledWith(
      { data: new Uint8Array([4, 5, 6]) },
      undefined,
    );
    expect(createObject).toHaveBeenCalledWith(
      "blog/site",
      {
        bucketId: "bucket-1",
        rootRef: {
          hash: {
            hashType: 1,
            hash: new Uint8Array([1, 2, 3]),
          },
        },
      },
      undefined,
    );
  });
});
