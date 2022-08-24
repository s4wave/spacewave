// CallbackPromise is a promise that is resolvable with a callback.
type CallbackPromise<T> = [Promise<T>, (val: T, err: unknown) => void]

// buildCallbackPromise builds a Promise that can be resolved with a callback.
export async function buildCallbackPromise<T>(): Promise<CallbackPromise<T>> {
  return new Promise<CallbackPromise<T>>((promResolve) => {
    const cbPromise = new Promise<T>((resolve, reject) => {
      promResolve([
        cbPromise,
        (val: T, err: unknown) => {
          if (err) {
            reject(err)
          } else {
            resolve(val)
          }
        },
      ])
    })
  })
}
