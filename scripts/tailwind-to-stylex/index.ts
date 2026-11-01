import { mkdir, readFile, writeFile, open } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'

import { createOracle } from './oracle.js'
import { transform } from './transform.js'
import { prepareComponentStyles, readSharedStyleApis } from './component-api.js'

// sourceFiles streams rg's source manifest without collecting the repository tree.
async function* sourceFiles(paths: string[]): AsyncGenerator<string> {
  // Generated bindings and tests retain their original source contracts during conversion.
  const command = Bun.spawn(
    [
      'rg',
      '--files',
      ...paths,
      '-g',
      '*.tsx',
      '-g',
      '*.ts',
      '-g',
      '!*.test.*',
      '-g',
      '!*.e2e.*',
      '-g',
      '!*.pb.ts',
      '-g',
      '!*.gen.ts',
      '-g',
      '!*.d.ts',
    ],
    { stdout: 'pipe', stderr: 'inherit' },
  )
  let pending = ''
  for await (const chunk of command.stdout) {
    pending += new TextDecoder().decode(chunk)
    const lines = pending.split('\n')
    pending = lines.pop()!
    for (const line of lines) if (line) yield line
  }
  if (pending) yield pending
  const code = await command.exited
  if (code > 1) throw new Error(`rg source manifest failed: ${code}`)
}

// main translates only the selected paths and keeps every unsupported site in its report.
async function main(): Promise<void> {
  // Dry runs are the default; --write changes exactly the requested source files.
  const args = process.argv.slice(2)
  const paths: string[] = []
  let write = false
  let componentApi = false
  let sharedSx = false
  let sharedCallers = false
  let reportPath: string | undefined

  // Select the source area and whether its component or caller contracts change.
  for (let index = 0; index < args.length; index++) {
    const argument = args[index]!
    if (argument === '--write') write = true
    else if (argument === '--component-api') componentApi = true
    else if (argument === '--shared-sx') sharedSx = true
    else if (argument === '--shared-callers') sharedSx = sharedCallers = true
    else if (argument === '--report') reportPath = args[++index]
    else if (argument.startsWith('-'))
      throw new Error(`Unknown option: ${argument}`)
    else paths.push(argument)
  }
  if (!paths.length || args.at(-1) === '--report')
    throw new Error(
      'Usage: bun run stylex:migrate [--write] [--component-api] [--shared-sx|--shared-callers] [--report file.jsonl] <paths...>',
    )

  // One compiler instance owns the theme and property-registration inventory for this run.
  const oracle = await createOracle(resolve('web/style/app.css'))
  if (reportPath) await mkdir(dirname(reportPath), { recursive: true })
  const report = reportPath ? await open(reportPath, 'w') : undefined

  // Keep only counts and one report row while the source manifest streams.
  const reasons = new Map<string, number>()
  let files = 0
  let changed = 0
  let converted = 0
  let manual = 0
  let apis = 0

  // Validate idempotence before writing a file; a partial translation never drops a class.
  try {
    for await (const file of sourceFiles(paths)) {
      const source = await readFile(file, 'utf8')
      if (
        !source.includes('className') &&
        !(sharedSx && source.includes('@stylexjs/stylex'))
      )
        continue
      const sharedComponents = sharedSx
        ? await readSharedStyleApis(source, file)
        : undefined
      const migrate = (input: string) => {
        const prepared = prepareComponentStyles(input, file, {
          convertProps: componentApi,
          sharedComponents,
        })
        let result = transform(prepared.source, file, oracle, {
          components: prepared.components,
          componentProps: componentApi,
          onlyComponents: sharedCallers,
        })
        if (prepared.converted && result.remaining.length) {
          const legacy = prepareComponentStyles(input, file, {
            convertProps: false,
            sharedComponents,
          })
          result = transform(input, file, oracle, {
            components: legacy.components,
            onlyComponents: sharedCallers,
          })
          return { ...result, apis: 0 }
        }
        return { ...result, apis: prepared.converted }
      }
      const result = migrate(source)
      if (migrate(result.source).source !== result.source)
        throw new Error(`Non-idempotent translation: ${file}`)
      files++
      converted += result.converted
      manual += result.remaining.length
      apis += result.apis
      if (source !== result.source) {
        changed++
        if (write) await writeFile(file, result.source)
      }
      for (const site of result.remaining)
        reasons.set(site.reason, (reasons.get(site.reason) ?? 0) + 1)
      await report?.write(
        JSON.stringify({
          file,
          converted: result.converted,
          apis: result.apis,
          remaining: result.remaining,
        }) + '\n',
      )
    }

    // The final row carries the global property registrations required after Tailwind leaves.
    const summary = {
      files,
      changed,
      converted,
      manual,
      apis,
      componentApi,
      sharedSx,
      sharedCallers,
      write,
      reasons: Object.fromEntries([...reasons].sort()),
      globalRules: [...oracle.globalRules].sort(),
    }
    await report?.write(JSON.stringify({ summary }) + '\n')
    console.log(
      JSON.stringify(
        { ...summary, globalRules: summary.globalRules.length },
        null,
        2,
      ),
    )
  } finally {
    await report?.close()
  }
}

if (import.meta.main) await main()
