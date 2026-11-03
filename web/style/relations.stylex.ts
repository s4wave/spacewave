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

/** traceLineMarker scopes running title effects to their own trace line. */
export const traceLineMarker = stylex.defineMarker()
/** traceTitleMarker identifies the preceding title beside a trailing trace icon. */
export const traceTitleMarker = stylex.defineMarker()

/** buttonGroupMarker scopes direct control focus and input layout. */
export const buttonGroupMarker = stylex.defineMarker()
/** buttonGroupHorizontalMarker joins horizontal control edges. */
export const buttonGroupHorizontalMarker = stylex.defineMarker()
/** buttonGroupVerticalMarker joins vertical control edges. */
export const buttonGroupVerticalMarker = stylex.defineMarker()

/** commandDialogMarker scopes dialog overrides to their actual command descendants. */
export const commandDialogMarker = stylex.defineMarker()
/** commandGroupMarker identifies preceding visible command groups. */
export const commandGroupMarker = stylex.defineMarker()
