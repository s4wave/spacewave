import type { Quad } from '@go/github.com/s4wave/spacewave/db/block/quad/quad.pb.js'
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'

import {
  graphLinkPredicatePolicy,
  type GraphLinkPredicatePolicy,
} from './graphLinkPolicy.js'
import type {
  CanvasNodeData,
  EphemeralEdge,
  HiddenGraphLinkData,
} from './types.js'

export interface SelectedGraphNode {
  nodeId: string
  node: CanvasNodeData
  iri: string
}

export interface GraphLookupResult {
  selected: SelectedGraphNode
  outgoing: Quad[]
  incoming: Quad[]
  outgoingTruncated?: boolean
  incomingTruncated?: boolean
}

export interface GraphLinkObjectMetadata {
  label: string
  type?: string
  typeLabel?: string
}

export interface BuildGraphLinkViewModelOptions {
  hiddenGraphLinks?: HiddenGraphLinkData[]
  objectMetadata?: Map<string, GraphLinkObjectMetadata>
  policy?: GraphLinkPredicatePolicy
}

function graphLinkRenderKey(
  sourceNodeId: string,
  direction: 'out' | 'in',
  subject: string,
  predicate: string,
  object: string,
  label: string,
): string {
  return JSON.stringify([
    sourceNodeId,
    direction,
    subject,
    predicate,
    object,
    label,
  ])
}

function graphLinkIdentityKey(link: HiddenGraphLinkData): string {
  return JSON.stringify([
    link.subject,
    link.predicate,
    link.object,
    link.label ?? '',
  ])
}

export function getSelectedGraphNodes(
  selectedNodeIds: Set<string>,
  nodes: Map<string, CanvasNodeData>,
): SelectedGraphNode[] {
  const selected: SelectedGraphNode[] = []
  for (const nodeId of selectedNodeIds) {
    const node = nodes.get(nodeId)
    if (node && node.type === 'world_object' && node.objectKey) {
      selected.push({
        nodeId,
        node,
        iri: keyToIRI(node.objectKey),
      })
    }
  }
  return selected
}

export function buildGraphLinkViewModel(
  results: GraphLookupResult[],
  nodesByObjectKey: Map<string, string>,
  opts: BuildGraphLinkViewModelOptions = {},
): EphemeralEdge[] {
  const edges: EphemeralEdge[] = []
  const hidden = new Set(
    (opts.hiddenGraphLinks ?? []).map(graphLinkIdentityKey),
  )
  const emitted = new Set<string>()
  const policy = opts.policy ?? graphLinkPredicatePolicy

  results.forEach(
    (
      { selected, outgoing, incoming, outgoingTruncated, incomingTruncated },
      sourceGroupIndex,
    ) => {
      const quads = [...outgoing, ...incoming]
      let sourceGroupOffset = 0
      let hiddenCount = 0
      const visible: EphemeralEdge[] = []

      for (const q of quads) {
        const subject = q.subject ?? ''
        const predicate = q.predicate ?? ''
        const object = q.obj ?? ''
        const label = q.label ?? ''
        if (!subject || !predicate || !object) continue
        if (subject !== selected.iri && object !== selected.iri) continue

        const identity: HiddenGraphLinkData = {
          subject,
          predicate,
          object,
          label: label || undefined,
        }
        if (hidden.has(graphLinkIdentityKey(identity))) {
          hiddenCount += 1
          continue
        }
        const identityKey = graphLinkIdentityKey(identity)
        if (emitted.has(identityKey)) continue

        const policyResult = policy.classify(identity)
        if (!policyResult.viewable) continue
        emitted.add(identityKey)

        const direction = subject === selected.iri ? 'out' : 'in'
        const linkedIri = direction === 'out' ? object : subject
        const linkedObjectKey = iriToKey(linkedIri)
        const metadata = opts.objectMetadata?.get(linkedObjectKey)
        const targetNodeId = nodesByObjectKey.get(linkedObjectKey)
        const stubOffset = sourceGroupOffset * 60 + 150

        visible.push({
          renderKey: graphLinkRenderKey(
            selected.nodeId,
            direction,
            subject,
            predicate,
            object,
            label,
          ),
          subject,
          predicate,
          object,
          label: label || undefined,
          sourceNodeId: selected.nodeId,
          sourceObjectKey: selected.node.objectKey ?? '',
          sourceGroupKey: selected.nodeId,
          sourceGroupIndex,
          sourceGroupOffset,
          outgoingTruncated: outgoingTruncated ?? false,
          incomingTruncated: incomingTruncated ?? false,
          hiddenCount: 0,
          direction,
          linkedObjectKey,
          linkedObjectLabel: metadata?.label ?? linkedObjectKey,
          linkedObjectType: metadata?.type,
          linkedObjectTypeLabel: metadata?.typeLabel,
          hideable: policyResult.hideable,
          userRemovable: policyResult.userRemovable,
          protected: policyResult.protected,
          ownerManaged: policyResult.ownerManaged,
          targetNodeId,
          stubX:
            targetNodeId ? undefined : (
              selected.node.x + selected.node.width + stubOffset
            ),
          stubY:
            targetNodeId ? undefined : (
              selected.node.y + selected.node.height / 2
            ),
        })
        sourceGroupOffset += 1
      }
      for (const edge of visible) {
        edges.push({ ...edge, hiddenCount })
      }
    },
  )
  return edges
}
