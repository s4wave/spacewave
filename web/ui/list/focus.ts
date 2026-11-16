/** isEditableElement reports whether el takes typed text. */
export function isEditableElement(el: Element | null): el is HTMLElement {
  return (
    el instanceof HTMLElement &&
    (el.matches('input, textarea, select, [contenteditable="true"]') ||
      el.isContentEditable)
  )
}
