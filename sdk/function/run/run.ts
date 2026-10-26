import type { MessageType } from '@aptre/protobuf-es-lite/message'
import { QuickJS, buildFileSystem } from 'quickjs-wasi-reactor'

import { harness } from './harness.gen.js'
import type { PureFunction } from '../function.pb.js'
import {
  DefaultInterruptChecks,
  DefaultMemoryLimitBytes,
  ModuleName,
} from '../function.js'

// InputPlaceholder is the token in the harness that a runner replaces with the
// input in protobuf JSON form.
const InputPlaceholder = '__PURE_FUNCTION_INPUT__'

// harnessName is the module name of the harness.
const harnessName = '/harness.js'

// PureFunctionError is an error the function threw or returned.
export class PureFunctionError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'PureFunctionError'
  }
}

// InstructionBudgetError is thrown when a run passes its interrupt checks.
export class InstructionBudgetError extends Error {
  constructor() {
    super('the function ran past its instruction budget')
    this.name = 'InstructionBudgetError'
  }
}

// MemoryBudgetError is thrown when a run passes its memory limit.
export class MemoryBudgetError extends Error {
  constructor() {
    super('the function ran past its memory budget')
    this.name = 'MemoryBudgetError'
  }
}

// HarnessResult is the line the harness prints.
interface HarnessResult {
  output?: unknown
  error?: string
  outOfMemory?: boolean
}

// runPureFunction runs fn on input in a fresh QuickJS instance of the reactor
// wasm and returns its output. The Go runner in this package evaluates the same
// harness in the same reactor build, so the same input gives the same output.
export async function runPureFunction<I, O>(
  wasm: WebAssembly.Module,
  fn: PureFunction,
  inputType: MessageType<I>,
  input: I,
  outputType: MessageType<O>,
): Promise<O> {
  const inputJson = inputType.toJsonString(input)
  const outputJson = await runPureFunctionJson(wasm, fn, inputJson)
  return outputType.fromJsonString(outputJson)
}

// runPureFunctionJson runs fn on the protobuf JSON input and returns its output
// as JSON.
export async function runPureFunctionJson(
  wasm: WebAssembly.Module,
  fn: PureFunction,
  inputJson: string,
): Promise<string> {
  // Check the function and its input.
  if (!fn.module) {
    throw new Error('the function has no compiled module')
  }
  if (!fn.inputType || !fn.outputType) {
    throw new Error('the function needs an input and an output type')
  }
  JSON.parse(inputJson)

  // Start an instance whose only file is the function's module.
  const stdout: string[] = []
  const stderr: string[] = []
  const memLimit = fn.budget?.memoryLimitBytes || DefaultMemoryLimitBytes
  const qjs = new QuickJS(wasm, {
    stdout: (line) => stdout.push(line),
    stderr: (line) => stderr.push(line),
    fs: buildFileSystem(new Map([[ModuleName, fn.module]])),
  })
  try {
    // The --memory-limit option reads a bare number as KiB.
    qjs.init(['qjs', '--memory-limit', `${memLimit}b`])

    // Run the harness under the instruction budget.
    qjs.setInterruptBudget(fn.budget?.interruptChecks || DefaultInterruptChecks)
    const source = harness.replace(InputPlaceholder, () => inputJson)
    let runErr: unknown
    try {
      qjs.eval(source, true, harnessName)
      await qjs.runLoop()
    } catch (err) {
      runErr = err
    }
    if (qjs.getInterruptBudget() === 0n) {
      throw new InstructionBudgetError()
    }
    return parseResult(stdout, stderr.join('\n'), runErr)
  } finally {
    qjs.destroy()
  }
}

// parseResult reads the harness's last stdout line.
function parseResult(
  stdout: string[],
  stderr: string,
  runErr: unknown,
): string {
  // A run that printed nothing failed in the harness itself.
  const line = stdout.findLast((l) => l.trim() !== '')
  if (!line) {
    if (stderr.includes('out of memory')) {
      throw new MemoryBudgetError()
    }
    const cause = runErr instanceof Error ? runErr.message : 'no result'
    throw new Error(stderr.trim() || `the function returned ${cause}`)
  }

  // Decode the result line.
  const res = JSON.parse(line) as HarnessResult
  if (res.outOfMemory) {
    throw new MemoryBudgetError()
  }
  if (res.error) {
    throw new PureFunctionError(res.error)
  }
  return JSON.stringify(res.output ?? {})
}
