#!/usr/bin/env bun

import { existsSync, readFileSync, writeFileSync } from 'fs'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const bldrStarPath = join(__dirname, '..', 'bldr.star')
const bldrYamlPath = join(__dirname, '..', 'bldr.yaml')

function incrementRev(line) {
  const match = line.match(/\brev=(\d+)/)
  if (!match) return { line, updated: false }
  const rev = match[1] ?? ''
  const nextRev = (parseInt(rev, 10) + 1).toString()
  console.log(`Incremented spacewave-app rev: ${rev} -> ${nextRev}`)
  return {
    line: line.replace(/\brev=\d+/, `rev=${nextRev}`),
    updated: true,
  }
}

function updateBldrStar(content) {
  const lines = content.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? ''
    if (!line.includes('"spacewave-app"')) continue
    if (!line.includes('js_plugin(') && !line.includes('manifest(')) continue

    const sameLine = incrementRev(line)
    if (sameLine.updated) {
      lines[i] = sameLine.line
      return lines.join('\n')
    }

    for (let j = i + 1; j < lines.length; j++) {
      const nextLine = lines[j] ?? ''
      if (nextLine.includes('manifest(') || nextLine.includes('js_plugin(')) {
        break
      }
      const updated = incrementRev(nextLine)
      if (!updated.updated) continue
      lines[j] = updated.line
      return lines.join('\n')
    }
  }
  return null
}

function updateBldrYaml(content) {
  return content.replace(
    /(spacewave-app:\s+builder:\s+id: bldr\/plugin\/compiler\/js\s+rev: )(\d+)/,
    (_match, prefix, rev) => {
      const nextRev = parseInt(rev, 10) + 1
      console.log(`Incremented spacewave-app rev: ${rev} -> ${nextRev}`)
      return prefix + nextRev
    },
  )
}

function writeUpdatedFile(path, updatedContent, label) {
  const content = readFileSync(path, 'utf-8')
  if (content === updatedContent) {
    console.error(`Failed to find and increment spacewave-app rev in ${label}`)
    process.exit(1)
  }
  writeFileSync(path, updatedContent, 'utf-8')
}

function main() {
  if (existsSync(bldrStarPath)) {
    const content = readFileSync(bldrStarPath, 'utf-8')
    const updatedContent = updateBldrStar(content)
    if (!updatedContent) {
      console.error('Failed to find and increment spacewave-app rev in bldr.star')
      process.exit(1)
    }
    writeUpdatedFile(bldrStarPath, updatedContent, 'bldr.star')
    return
  }

  if (existsSync(bldrYamlPath)) {
    const content = readFileSync(bldrYamlPath, 'utf-8')
    writeUpdatedFile(bldrYamlPath, updateBldrYaml(content), 'bldr.yaml')
    return
  }

  console.error('Failed to find bldr.star or bldr.yaml')
  process.exit(1)
}

main()
