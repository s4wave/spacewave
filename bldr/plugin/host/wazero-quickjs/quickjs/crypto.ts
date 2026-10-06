import type { QuickjsGlobalScope } from "./quickjs.js";

// randomDevicePath is the host device that streams secure random bytes.
const randomDevicePath = "/dev/urandom";

// maxRandomValuesBytes is the Web Crypto limit for one getRandomValues call.
const maxRandomValuesBytes = 65536;

// QuickjsCrypto is the subset of Web Crypto the QuickJS polyfill provides.
export interface QuickjsCrypto {
  getRandomValues<T extends ArrayBufferView | null>(array: T): T;
}

/**
 * createQuickjsCrypto implements crypto.getRandomValues by reading the host's
 * random device, because QuickJS has no secure random source of its own. The
 * device opens on first use and stays open for the life of the plugin.
 *
 * @param os - QuickJS os module instance
 */
export function createQuickjsCrypto(
  os: QuickjsGlobalScope["os"],
): QuickjsCrypto {
  let fd: number | undefined;
  return {
    getRandomValues<T extends ArrayBufferView | null>(array: T): T {
      // Validate the target like Web Crypto.
      if (array === null) {
        throw new TypeError("getRandomValues requires a typed array");
      }
      if (array.byteLength > maxRandomValuesBytes) {
        throw new RangeError(
          `getRandomValues length ${array.byteLength} exceeds ${maxRandomValuesBytes} bytes`,
        );
      }

      // Open the random device once.
      if (fd === undefined) {
        const opened = os.open(randomDevicePath, os.O_RDONLY);
        if (opened < 0) {
          throw new Error(`open ${randomDevicePath}: error code ${opened}`);
        }
        fd = opened;
      }

      // Fill the whole view, which a device read may return in parts.
      let offset = 0;
      while (offset < array.byteLength) {
        const n = os.read(
          fd,
          array.buffer as ArrayBuffer,
          array.byteOffset + offset,
          array.byteLength - offset,
        );
        if (n <= 0) {
          throw new Error(`read ${randomDevicePath}: error code ${n}`);
        }
        offset += n;
      }
      return array;
    },
  };
}
