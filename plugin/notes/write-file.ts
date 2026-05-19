import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

export async function createTextFile(
  parent: FSHandle,
  name: string,
  content: string,
): Promise<void> {
  const data = new TextEncoder().encode(content)
  await parent.uploadFile(
    name,
    BigInt(data.byteLength),
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(data)
        controller.close()
      },
    }),
  )
}
