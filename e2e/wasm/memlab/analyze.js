import { getFullHeapFromFile } from '@memlab/heap-analysis'

// Parse CLI args: --snapshots label1=path1,label2=path2,...
function parseArgs() {
  const idx = process.argv.indexOf('--snapshots')
  if (idx === -1 || idx + 1 >= process.argv.length) {
    throw new Error('usage: node analyze.js --snapshots label1=path1,label2=path2')
  }
  const pairs = process.argv[idx + 1].split(',')
  return pairs.map((p) => {
    const eq = p.indexOf('=')
    if (eq === -1) throw new Error(`bad snapshot arg: ${p}`)
    return { label: p.slice(0, eq), path: p.slice(eq + 1) }
  })
}

async function analyzeSnapshot(label, filePath) {
  const heap = await getFullHeapFromFile(filePath)
  const counts = {
    clientRpc: 0,
    channelStream: 0,
    promise: 0,
    generator: 0,
    onNext: 0,
  }
  const clientRpcPairs = new Map()

  heap.nodes.forEach((node) => {
    const name = node.name

    if (name === 'ClientRPC') {
      counts.clientRpc++
      const svc = node.getReferenceNode('service', 'property') || node.getReferenceNode('service')
      const mtd = node.getReferenceNode('method', 'property') || node.getReferenceNode('method')
      const pair = `${svc?.name || '?'}/${mtd?.name || '?'}`
      clientRpcPairs.set(pair, (clientRpcPairs.get(pair) || 0) + 1)
      return
    }
    if (name === 'ChannelStream') {
      counts.channelStream++
      return
    }
    if (name === 'Promise') {
      counts.promise++
      if (node.referrers.some((e) => e.fromNode?.name === 'onNext')) {
        counts.onNext++
      }
      return
    }
    if (name === 'Generator') {
      counts.generator++
      return
    }
    if (node.type === 'closure' && name === 'onNext') {
      counts.onNext++
    }
  })

  const topRetained = [...clientRpcPairs.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 20)
    .map(([key, count]) => {
      const slash = key.lastIndexOf('/')
      return { service: key.slice(0, slash), method: key.slice(slash + 1), count }
    })

  return {
    label,
    path: filePath,
    counts,
    topRetained,
    pairCounts: Object.fromEntries(clientRpcPairs),
  }
}

function buildPairDeltas(firstPairs, lastPairs) {
  const allKeys = new Set([
    ...Object.keys(firstPairs || {}),
    ...Object.keys(lastPairs || {}),
  ])
  return [...allKeys]
    .map((key) => {
      const baselineCount = firstPairs?.[key] || 0
      const finalCount = lastPairs?.[key] || 0
      const slash = key.lastIndexOf('/')
      return {
        service: key.slice(0, slash),
        method: key.slice(slash + 1),
        baselineCount,
        finalCount,
        delta: finalCount - baselineCount,
      }
    })
    .filter((entry) => entry.delta !== 0)
    .sort((a, b) => {
      if (a.delta !== b.delta) {
        return b.delta - a.delta
      }
      if (a.finalCount !== b.finalCount) {
        return b.finalCount - a.finalCount
      }
      return `${a.service}/${a.method}`.localeCompare(`${b.service}/${b.method}`)
    })
    .slice(0, 20)
}

async function main() {
  const inputs = parseArgs()
  const snapshots = []
  for (const { label, path } of inputs) {
    snapshots.push(await analyzeSnapshot(label, path))
  }

  // Compute deltas (last - first).
  const first = snapshots[0]?.counts || {}
  const last = snapshots[snapshots.length - 1]?.counts || {}
  const deltas = {}
  for (const key of Object.keys(first)) {
    deltas[key] = (last[key] || 0) - (first[key] || 0)
  }

  const result = {
    snapshots,
    deltas,
    topRetained: snapshots[snapshots.length - 1]?.topRetained || [],
    pairDeltas: buildPairDeltas(
      snapshots[0]?.pairCounts,
      snapshots[snapshots.length - 1]?.pairCounts,
    ),
  }
  process.stdout.write(JSON.stringify(result))
}

main().catch((err) => {
  process.stderr.write(err.message + '\n')
  process.exitCode = 1
})
