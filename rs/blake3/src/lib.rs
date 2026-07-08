#![no_std]

use core::arch::wasm32;
use core::panic::PanicInfo;
use core::{slice, str};

const PAGE_SIZE: usize = 65_536;

extern "C" {
    static __heap_base: u8;
}

static mut WORKSPACE_CAP: usize = 0;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    loop {}
}

#[no_mangle]
pub extern "C" fn blake3_workspace(len: usize) -> *mut u8 {
    let ptr = unsafe { &__heap_base as *const u8 as usize };
    let required = ptr.saturating_add(len);
    let current = wasm32::memory_size(0) * PAGE_SIZE;
    if required > current {
        let additional = required - current;
        let pages = additional.div_ceil(PAGE_SIZE);
        let previous = wasm32::memory_grow(0, pages);
        if previous == usize::MAX {
            return core::ptr::null_mut();
        }
    }
    unsafe {
        if WORKSPACE_CAP < len {
            WORKSPACE_CAP = len;
        }
    }
    ptr as *mut u8
}

#[no_mangle]
pub unsafe extern "C" fn blake3_hash(
    data_ptr: *const u8,
    data_len: usize,
    out_ptr: *mut u8,
    out_len: usize,
) {
    let data = slice::from_raw_parts(data_ptr, data_len);
    let out = slice::from_raw_parts_mut(out_ptr, out_len);
    let mut hasher = blake3::Hasher::new();
    hasher.update(data);
    hasher.finalize_xof().fill(out);
}

#[no_mangle]
pub unsafe extern "C" fn blake3_keyed_hash(
    key_ptr: *const u8,
    data_ptr: *const u8,
    data_len: usize,
    out_ptr: *mut u8,
    out_len: usize,
) {
    let mut key = [0u8; 32];
    key.copy_from_slice(slice::from_raw_parts(key_ptr, 32));
    let data = slice::from_raw_parts(data_ptr, data_len);
    let out = slice::from_raw_parts_mut(out_ptr, out_len);
    let mut hasher = blake3::Hasher::new_keyed(&key);
    hasher.update(data);
    hasher.finalize_xof().fill(out);
}

#[no_mangle]
pub unsafe extern "C" fn blake3_derive_key(
    context_ptr: *const u8,
    context_len: usize,
    material_ptr: *const u8,
    material_len: usize,
    out_ptr: *mut u8,
    out_len: usize,
) {
    let context_bytes = slice::from_raw_parts(context_ptr, context_len);
    let context = str::from_utf8_unchecked(context_bytes);
    let material = slice::from_raw_parts(material_ptr, material_len);
    let out = slice::from_raw_parts_mut(out_ptr, out_len);
    let mut hasher = blake3::Hasher::new_derive_key(context);
    hasher.update(material);
    hasher.finalize_xof().fill(out);
}
