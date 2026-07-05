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

function output(command: string, args: string[]): string {
  const result = spawnSync(command, args, {
    cwd: process.cwd(),
    encoding: 'utf8',
  })
  if (result.status !== 0) {
    process.stderr.write(result.stderr)
    throw new Error(`${command} ${args.join(' ')} exited ${result.status}`)
  }
  return result.stdout.trim()
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

  const commonDir = output('go', ['list', '-m', '-f', '{{.Dir}}', commonModule])
  const staleFiles = generatedFiles.filter(({ actual, expected }) => {
    const actualPath = join(toolsDir, actual)
    const expectedPath = join(commonDir, expected)
    if (!existsSync(actualPath)) {
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
