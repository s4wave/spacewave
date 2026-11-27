package alarm_volume

// seriesTypes declares the parts of the function input the store health
// functions read, and labelOf, which reads one label of a series.
const seriesTypes = `
interface Label { name?: string; value?: string }
interface Series { labels?: { labels?: Label[] }; samples?: { value?: number }[] }
interface Input { queries?: Record<string, { series?: Series[] }> }

function labelOf(s: Series, name: string): string {
  return s.labels?.labels?.find((l) => l.name === name)?.value ?? ''
}

function valueOf(s: Series): number {
  return (s.samples ?? []).at(-1)?.value ?? 0
}

function describe(series: Series[], line: (s: Series) => string): string {
  return series.map(line).sort().join('; ')
}
`

// gcStuckSource fires while the stuck query returns a volume, listing each
// volume and its phase.
const gcStuckSource = seriesTypes + `
export default function volumeGcStuck(input: Input) {
  const stuck = input.queries?.stuck?.series ?? []
  if (stuck.length === 0) {
    return { summary: 'no volume is stuck in a collection phase' }
  }
  return {
    firing: true,
    summary: describe(stuck, (s) => labelOf(s, 'volume') + ' stuck in ' + labelOf(s, 'phase')),
  }
}
`

// reserveSoonSource fires while the hours query returns a volume, listing
// each volume and the whole hours until it reaches its reserve.
const reserveSoonSource = seriesTypes + `
export default function volumeReserveSoon(input: Input) {
  const soon = input.queries?.hours?.series ?? []
  if (soon.length === 0) {
    return { summary: 'no volume is forecast to reach its reserve' }
  }
  return {
    firing: true,
    summary: describe(soon, (s) => labelOf(s, 'volume') + ' reaches its reserve in ' + Math.ceil(valueOf(s)) + ' h'),
  }
}
`

// reserveSource fires while the reserve query returns a volume, listing each
// volume that refuses writes.
const reserveSource = seriesTypes + `
export default function volumeReserve(input: Input) {
  const full = input.queries?.reserve?.series ?? []
  if (full.length === 0) {
    return { summary: 'every volume accepts writes' }
  }
  return {
    firing: true,
    summary: describe(full, (s) => labelOf(s, 'volume') + ' refuses writes below its reserve'),
  }
}
`
