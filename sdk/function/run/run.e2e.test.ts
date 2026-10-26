import { describe, expect, it } from 'vitest'

import { LabelSet, QueryResponse } from '../../prom/prom.pb.js'
import type { PureFunction } from '../function.pb.js'
import fixture from './testdata/down-instances.json'
import {
  InstructionBudgetError,
  MemoryBudgetError,
  PureFunctionError,
  runPureFunction,
  runPureFunctionJson,
} from './run.js'

// loadReactor compiles the reactor wasm the Go runner embeds.
async function loadReactor(): Promise<WebAssembly.Module> {
  const url = new URL(
    '../../../node_modules/quickjs-wasi-reactor/qjs-wasi.wasm',
    import.meta.url,
  )
  return WebAssembly.compileStreaming(fetch(url))
}

// jsFunction returns a PureFunction whose module is plain JavaScript.
function jsFunction(module: string, budget?: PureFunction['budget']) {
  return {
    module,
    inputType: 's4wave.prom.QueryResponse',
    outputType: 's4wave.prom.LabelSet',
    budget,
  }
}

describe('runPureFunction', () => {
  it('gives the output the Go runner gives', async () => {
    // Run the fixture the Go run-through test checks.
    const wasm = await loadReactor()
    const fn: PureFunction = {
      source: fixture.source,
      module: fixture.module,
      inputType: fixture.inputType,
      outputType: fixture.outputType,
    }
    const input = QueryResponse.fromJson(fixture.input)
    const out = await runPureFunction(wasm, fn, QueryResponse, input, LabelSet)

    // The output matches the fixture, which the Go runner wrote.
    expect(LabelSet.toJson(out)).toEqual(
      LabelSet.toJson(LabelSet.fromJson(fixture.output)),
    )
    expect(out.labels?.map((l) => l.value)).toEqual([
      '1970-01-01T00:00:00.000Z',
      'forge-ü:9100',
      '0.358889980',
    ])
  })

  it('fails a run past its instruction budget', async () => {
    const wasm = await loadReactor()
    const fn = jsFunction(
      'export default () => { try { for (;;) {} } catch {} return {} }',
      { interruptChecks: 10 },
    )
    await expect(runPureFunctionJson(wasm, fn, '{}')).rejects.toBeInstanceOf(
      InstructionBudgetError,
    )
  })

  it('fails a run past its memory budget', async () => {
    // A function that holds about 8 MiB passes a 4 MiB heap limit and fits a
    // 64 MiB limit.
    const wasm = await loadReactor()
    const hold =
      "export default () => { const a = []; for (let i = 0; i < 128; i++) a.push(new Array(4096).fill(i)); return { labels: [{ name: 'n', value: String(a.length) }] } }"
    const small = jsFunction(hold, { memoryLimitBytes: 4 << 20 })
    await expect(runPureFunctionJson(wasm, small, '{}')).rejects.toBeInstanceOf(
      MemoryBudgetError,
    )
    const large = jsFunction(hold, { memoryLimitBytes: 64 << 20 })
    expect(await runPureFunctionJson(wasm, large, '{}')).toContain('"128"')
  })

  it('reports an error the function throws', async () => {
    const wasm = await loadReactor()
    const fn = jsFunction(
      "export default () => { throw new RangeError('no samples') }",
    )
    await expect(runPureFunctionJson(wasm, fn, '{}')).rejects.toEqual(
      new PureFunctionError('RangeError: no samples'),
    )
  })
})
