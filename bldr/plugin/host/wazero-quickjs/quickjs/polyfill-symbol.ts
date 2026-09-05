import '../../../../sdk/dispose-symbol.js'

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
}
