import { spawnSync } from 'node:child_process'
import { existsSync, lstatSync, readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'

const commonModule = 'github.com/aperturerobotics/common'

const generatedFiles = [
  { actual: '.gitignore', expected: '.gitignore' },
  { actual: '.oxfmtrc.json', expected: '.oxfmtrc.json' },
  { actual: '.oxlintrc.json', expected: '.oxlintrc.json' },
  { actual: 'deps.go', expected: 'deps.go.tools' },
  { actual: 'go.mod', expected: 'go.mod.tools' },
  { actual: 'go.sum', expected: 'go.sum.tools' },
  { actual: 'tsconfig.json', expected: 'tsconfig.json' },
]

function run(command: string, args: string[]) {
  const result = spawnSync(command, args, {
    cwd: process.cwd(),
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} exited ${result.status}`)
  }
}

function removeTools(reason: string) {
  console.log(`prepare: removing .tools: ${reason}`)
  rmSync(join(process.cwd(), '.tools'), { recursive: true, force: true })
}

function validateTools() {
  const repoRoot = process.cwd()
  const toolsDir = join(repoRoot, '.tools')
  if (!existsSync(toolsDir)) {
    console.log('prepare: .tools absent; aptre will create it on demand')
    return
  }
  if (!lstatSync(toolsDir).isDirectory()) {
    removeTools('path is not a directory')
    return
  }

  // Compare against the vendored common module: go mod vendor has already run,
  // so vendor/ always holds the pinned version, unlike the module cache.
  const commonDir = join(repoRoot, 'vendor', commonModule)
  if (!existsSync(commonDir)) {
    removeTools(`${commonModule} missing from vendor`)
    return
  }
  const staleFiles = generatedFiles.filter(({ actual, expected }) => {
    const actualPath = join(toolsDir, actual)
    const expectedPath = join(commonDir, expected)
    if (!existsSync(actualPath) || !existsSync(expectedPath)) {
      return true
    }
    return !readFileSync(actualPath).equals(readFileSync(expectedPath))
  })

  if (staleFiles.length !== 0) {
    removeTools(
      `generated files differ from ${commonModule}: ${staleFiles
        .map(({ actual }) => actual)
        .join(', ')}`,
    )
    return
  }

  console.log('prepare: reusing .tools; generated files match aptre common')
}

run('go', ['mod', 'vendor'])
validateTools()
run('bun', ['run', 'setup'])
run('git', ['config', 'core.hooksPath', '.githooks'])
