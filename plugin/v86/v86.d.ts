declare module '@aptre/v86' {
  export type KeyEvent = Pick<
    KeyboardEvent,
    'code' | 'key' | 'keyCode' | 'altKey' | 'shiftKey' | 'ctrlKey' | 'repeat'
  >

  export class V86 {
    constructor(options: Record<string, unknown>)
    // v86 is the emulator internals; the runner reaches the VirtioV86FS
    // device through it to push host notifications into the guest.
    v86: {
      cpu: {
        devices: {
          virtio_v86fs: {
            invalidate_inode_range(
              inode_id: number,
              offset: number,
              size: number,
            ): void
            invalidate_dir(inode_id: number): void
            mount_notify(name: string, mount_path: string): void
            umount_notify(mount_path: string): void
          }
        }
      }
    }
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
    keyboard_send_event(key: KeyEvent, down: boolean): void
    keyboard_release_all(): void
    keyboard_send_scancodes(codes: number[]): Promise<void>
    screen_fill_buffer(): void
    screen_redraw(): void
    run(): void
    stop(): void
    destroy(): void
  }
}

declare module '@aptre/v86/screen' {
  export interface ScreenOptions {
    container: HTMLElement
    scale?: number
    use_graphical_text?: boolean
    encoding?: string
  }

  export interface ScreenLayer {
    image_data: ImageData
    screen_x: number
    screen_y: number
    buffer_x: number
    buffer_y: number
    buffer_width: number
    buffer_height: number
  }

  export class ScreenAdapter {
    constructor(options: ScreenOptions, screen_fill_buffer: () => void)
    set_mode(graphical: boolean): void
    set_size_text(cols: number, rows: number): void
    set_size_graphical(
      width: number,
      height: number,
      buffer_width: number,
      buffer_height: number,
    ): void
    set_font_bitmap(
      height: number,
      width_9px: boolean,
      width_dbl: boolean,
      copy_8th_col: boolean,
      vga_bitmap: Uint8Array,
      vga_bitmap_changed: boolean,
    ): void
    set_font_page(page_a: number, page_b: number): void
    put_char(
      row: number,
      col: number,
      chr: number,
      flags: number,
      bg_color: number,
      fg_color: number,
    ): void
    update_cursor(row: number, col: number): void
    update_cursor_scanline(start: number, end: number, enabled: boolean): void
    clear_screen(): void
    update_buffer(layers: ScreenLayer[]): void
    destroy(): void
  }
}
