// codeKeysyms maps the physical keys whose side matters to their X keysyms.
const codeKeysyms: Record<string, number> = {
  ShiftLeft: 0xffe1,
  ShiftRight: 0xffe2,
  ControlLeft: 0xffe3,
  ControlRight: 0xffe4,
  CapsLock: 0xffe5,
  MetaLeft: 0xffeb,
  MetaRight: 0xffec,
  AltLeft: 0xffe9,
  AltRight: 0xffea,
}

// namedKeysyms maps the named keys of KeyboardEvent.key to their X keysyms.
const namedKeysyms: Record<string, number> = {
  Backspace: 0xff08,
  Tab: 0xff09,
  Enter: 0xff0d,
  Escape: 0xff1b,
  Home: 0xff50,
  ArrowLeft: 0xff51,
  ArrowUp: 0xff52,
  ArrowRight: 0xff53,
  ArrowDown: 0xff54,
  PageUp: 0xff55,
  PageDown: 0xff56,
  End: 0xff57,
  Insert: 0xff63,
  Delete: 0xffff,
}

// keysymOf returns the X keysym of a browser key event's key and code, or 0
// for a key X has no keysym for. A character below U+0100 is its own keysym.
// Any other character is its Unicode keysym.
export function keysymOf(key: string, code: string): number {
  const named = codeKeysyms[code] ?? namedKeysyms[key]
  if (named) return named
  const fn = /^F([1-9]|1[0-2])$/.exec(key)
  if (fn) return 0xffbe + Number(fn[1]) - 1
  const chars = [...key]
  if (chars.length !== 1) return 0
  const cp = chars[0].codePointAt(0) ?? 0
  if (cp < 0x20) return 0
  return cp < 0x100 ? cp : 0x01000000 + cp
}

// xButtons converts a PointerEvent.buttons mask, which holds the left, right
// and middle buttons in bits 0, 1 and 2, to the X mask, which holds buttons
// 1, 2 and 3 (left, middle and right) in bits 0, 1 and 2.
export function xButtons(buttons: number): number {
  return (buttons & 1) | ((buttons & 4) >> 1) | ((buttons & 2) << 1)
}
