import type { EphemeralEdge } from '../types.js'

interface GraphLinkWorld {
  deleteGraphQuad(
    subject: string,
    predicate: string,
    object: string,
    label?: string,
  ): Promise<void>
}

interface DeleteGraphLinkOptions {
  link: EphemeralEdge
  world: GraphLinkWorld | null | undefined
  onError: (message: string) => void
  onDeleted: () => void
}

// deleteCanvasGraphLink deletes a policy-approved world graph link.
export async function deleteCanvasGraphLink({
  link,
  world,
  onError,
  onDeleted,
}: DeleteGraphLinkOptions): Promise<void> {
  if (!world) {
    onError('Cannot delete graph link before the world state is loaded.')
    return
  }
  if (!link.userRemovable) {
    onError(
      'This graph link is owned by its object type and cannot be deleted here.',
    )
    return
  }

  try {
    await world.deleteGraphQuad(
      link.subject,
      link.predicate,
      link.object,
      link.label,
    )
    onDeleted()
  } catch {
    onError('Deleting the graph link failed. The link was not removed.')
  }
}
