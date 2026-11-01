// Package function_run_browser runs a PureFunction module with run.ts in a
// fresh QuickJS instance of the reactor on the browser's WebAssembly API.
import * as $ from '@goscript/builtin/index.js'
import type * as context from '@goscript/context/index.js'

import {
  InstructionBudgetError,
  MemoryBudgetError,
  PureFunctionError,
  runPureFunctionJson,
} from '@go/github.com/s4wave/spacewave/sdk/function/run/run.js'

// Outcome is how a run that reached the harness ended.
export type Outcome = number

// OutcomeOutput is a run whose result is the function's output.
export const OutcomeOutput: Outcome = 0
// OutcomeThrew is a run whose result is what the function threw.
export const OutcomeThrew: Outcome = 1
// OutcomeInstructionBudget is a run that passed its interrupt checks.
export const OutcomeInstructionBudget: Outcome = 2
// OutcomeMemoryBudget is a run that passed its memory limit.
export const OutcomeMemoryBudget: Outcome = 3

// compiled holds the compiled reactor of each reactor slice, so a Device
// compiles the reactor once however many functions it runs.
const compiled = new WeakMap<object, Promise<WebAssembly.Module>>()

// compileReactor returns the compiled reactor wasm.
function compileReactor(
  reactor: NonNullable<$.Slice<number>>,
): Promise<WebAssembly.Module> {
  let mod = compiled.get(reactor)
  if (!mod) {
    // GoScript byte slices view an ArrayBuffer, never shared memory.
    const bytes = (
      reactor instanceof Uint8Array
        ? reactor
        : new Uint8Array($.asArray(reactor))
    ) as Uint8Array<ArrayBuffer>
    mod = WebAssembly.compile(bytes)
    compiled.set(reactor, mod)
    mod.catch(() => compiled.delete(reactor))
  }
  return mod
}

// Run runs module's default export on the protobuf JSON input in a fresh
// instance of the reactor wasm, under the memory limit in bytes and the
// interrupt check limit. result is the output in protobuf JSON form or the
// thrown message, as outcome says; the error is a failure of the runner.
export async function Run(
  _ctx: context.Context | null,
  reactor: $.Slice<number>,
  module: string,
  memoryLimit: number,
  interruptLimit: number,
  input: string,
): Promise<[Outcome, string, $.GoError]> {
  if (!reactor) {
    return [OutcomeOutput, '', $.newError('the reactor wasm is empty')]
  }
  try {
    const wasm = await compileReactor(reactor)
    const output = await runPureFunctionJson(
      wasm,
      {
        source: '',
        module,
        inputType: 'input',
        outputType: 'output',
        budget: {
          memoryLimitBytes: memoryLimit,
          interruptChecks: interruptLimit,
        },
      },
      input,
    )
    return [OutcomeOutput, output, null]
  } catch (err) {
    if (err instanceof PureFunctionError) {
      return [OutcomeThrew, err.message, null]
    }
    if (err instanceof InstructionBudgetError) {
      return [OutcomeInstructionBudget, '', null]
    }
    if (err instanceof MemoryBudgetError) {
      return [OutcomeMemoryBudget, '', null]
    }
    return [
      OutcomeOutput,
      '',
      err instanceof Error ? $.toGoError(err) : $.newError(String(err)),
    ]
  }
}
