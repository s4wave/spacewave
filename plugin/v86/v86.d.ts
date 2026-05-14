declare module '@aptre/v86' {
  export class V86 {
    constructor(options: Record<string, unknown>)
    add_listener(
      event: 'serial0-output-byte',
      callback: (data: number) => void,
    ): void
    add_listener(event: string, callback: (data: unknown) => void): void
    remove_listener(
      event: 'serial0-output-byte',
      callback: (data: number) => void,
    ): void
    remove_listener(event: string, callback: (data: unknown) => void): void
    serial0_send(data: string): void
    run(): void
    stop(): void
    destroy(): void
  }
}
