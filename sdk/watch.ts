// firstWatchEmission returns the first value from a watch stream.
export async function firstWatchEmission<T>(
  stream: AsyncIterable<T>,
): Promise<T | null> {
  for await (const value of stream) {
    return value
  }
  return null
}
