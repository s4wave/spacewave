export function timeoutPromise(dur: number): Promise<void> {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, dur)
  })
}
