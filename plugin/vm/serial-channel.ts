// serial-channel.ts - VmV86 serial bridge channel naming and frame shape.
//
// The v86 backend (SharedWorker) posts guest-emitted bytes onto a
// BroadcastChannel keyed by the VmV86 object key; the VmV86Viewer subscribes
// to the same channel to render the bytes in an xterm terminal and posts
// user-typed text back as input frames.

// SerialFrame is the wire shape carried on the v86-serial broadcast channel.
// dir="out" frames carry a single guest-emitted byte; dir="in" frames carry
// text chunks to push into the emulator's COM1 port.
export interface SerialFrame {
  dir: 'out' | 'in'
  byte?: number
  text?: string
}

// v86SerialChannelName returns the BroadcastChannel name used to relay
// serial bytes for the VmV86 instance at =vmObjectKey=. Callers on both
// ends (backend and viewer) must use the same key so the BroadcastChannel
// addresses line up.
export function v86SerialChannelName(vmObjectKey: string): string {
  return `v86-serial-${vmObjectKey}`
}
