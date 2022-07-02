// castToError casts an object to an Error.
// if err is a string, uses it as the message.
// if err is undefined, returns new Error(defaultMsg)
export function castToError(err: any, defaultMsg?: string): Error {
  defaultMsg = defaultMsg || 'error'
  if (!err) {
    return new Error(defaultMsg)
  }
  if (typeof err === 'string') {
    return new Error(err)
  }
  const asError = err as Error
  if (asError.message) {
    return asError
  }
  if (err.toString) {
    const errString = err.toString()
    if (errString) {
      return new Error(errString)
    }
  }
  return new Error(defaultMsg)
}
