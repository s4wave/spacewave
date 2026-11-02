/** relationMarkers maps the existing utility marker classes to stable StyleX exports. */
export const relationMarkers: Readonly<Record<string, string>> = {
  group: 'groupMarker',
  peer: 'peerMarker',
  'group/code': 'codeGroupMarker',
  'group/code-block': 'codeBlockGroupMarker',
  'group/header': 'headerGroupMarker',
  'group/trace-collapsible-trigger': 'traceTriggerGroupMarker',
}

/** relationCondition accepts an ancestor or preceding-peer selector from the utility compiler. */
export function relationCondition(selector: string): string | undefined {
  // Recognize the compiler's exact marker, predicate and relationship shape.
  const match = selector.match(
    /^:is\(:where\(\.((?:\\.|[\w-])+)\)((?::[\w-]+|\[[^\]]+\])+)( ~)? \*\)$/,
  )
  if (!match) return

  // Only existing owned marker classes participate in automatic conversion.
  const marker = match[1]!.replace(/\\(.)/g, '$1')
  if (!relationMarkers[marker]) return
  const method = match[3] ? 'siblingBefore' : 'ancestor'
  if (
    method === 'siblingBefore' ? marker !== 'peer' : !marker.startsWith('group')
  )
    return
  return '@stylex-relation ' + JSON.stringify([method, marker, match[2]])
}
