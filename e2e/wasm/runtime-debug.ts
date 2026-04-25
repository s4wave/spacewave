// Runtime debug helpers for memlab integration.
// Works in both Page and Worker contexts via globalThis.__APTRE_RPC_DEBUG__.
export default function (args: {
  op: 'snapshot' | 'typeof'
}): string | null {
  const debug = (globalThis as Record<string, unknown>).__APTRE_RPC_DEBUG__ as
    | { snapshot?: () => unknown }
    | undefined
  if (args.op === 'typeof') {
    return typeof debug
  }
  return JSON.stringify(debug?.snapshot?.() ?? null)
}
