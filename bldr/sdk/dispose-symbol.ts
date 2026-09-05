// Install disposal symbols before classes declare their computed methods. The
// registry keys agree with the fallback used by lowered `using` declarations.
for (const name of ['dispose', 'asyncDispose'] as const) {
  if (!Symbol[name]) {
    Object.defineProperty(Symbol, name, {
      value: Symbol.for(`Symbol.${name}`),
    })
  }
}
