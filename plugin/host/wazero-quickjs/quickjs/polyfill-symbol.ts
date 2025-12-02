// createSymbolPolyfills adds missing Symbol properties if they don't exist.
export function createSymbolPolyfills(): void {
  if (!Symbol.asyncIterator) {
    Object.defineProperty(Symbol, 'asyncIterator', {
      value: Symbol('Symbol.asyncIterator'),
      writable: false,
      enumerable: false,
      configurable: false,
    })
  }

  if (!Symbol.dispose) {
    Object.defineProperty(Symbol, 'dispose', {
      value: Symbol('Symbol.dispose'),
      writable: false,
      enumerable: false,
      configurable: false,
    })
  }

  if (!Symbol.asyncDispose) {
    Object.defineProperty(Symbol, 'asyncDispose', {
      value: Symbol('Symbol.asyncDispose'),
      writable: false,
      enumerable: false,
      configurable: false,
    })
  }
}
