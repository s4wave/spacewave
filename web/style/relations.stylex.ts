import * as stylex from '@stylexjs/stylex'

/** groupMarker connects an owned ancestor to its descendant conditions. */
export const groupMarker = stylex.defineMarker()
/** peerMarker connects a preceding control to its following sibling conditions. */
export const peerMarker = stylex.defineMarker()
/** codeGroupMarker scopes actions to their code ancestor. */
export const codeGroupMarker = stylex.defineMarker()
/** codeBlockGroupMarker scopes actions to their code block ancestor. */
export const codeBlockGroupMarker = stylex.defineMarker()
/** headerGroupMarker scopes actions to their header ancestor. */
export const headerGroupMarker = stylex.defineMarker()
/** traceTriggerGroupMarker scopes expansion indicators to their trace trigger. */
export const traceTriggerGroupMarker = stylex.defineMarker()
