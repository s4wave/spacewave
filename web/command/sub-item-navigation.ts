const SUB_ITEM_QUERY_PREFIX = 'spacewave:sub-item-query:'

// createSubItemQueryId encodes a palette sub-item that updates the active query.
export function createSubItemQueryId(query: string): string {
  return `${SUB_ITEM_QUERY_PREFIX}${encodeURIComponent(query)}`
}

// getSubItemQuery decodes a palette query-navigation sub-item identifier.
export function getSubItemQuery(id: string): string | null {
  if (!id.startsWith(SUB_ITEM_QUERY_PREFIX)) return null
  try {
    return decodeURIComponent(id.slice(SUB_ITEM_QUERY_PREFIX.length))
  } catch {
    return null
  }
}
