#pragma once

#include <stddef.h>
#include <stdint.h>

// Native IPC boundary for zero-native WebRuntime transport probes.
//
// This proves a zero-native-owned C ABI can perform an echo round trip and
// callback packet streams over a native pipe/socket with explicit remote errors
// and clean close behavior. The backend Resource SDK transport invariant is
// StarPC framing over native IPC; renderer-local transports such as MessagePort,
// shared-worker, in-process calls, or direct JavaScript callbacks are not
// substitutes for this backend boundary. It is not a renderer boot path, package
// hook, or full WebRuntime transport.

#define SPACEWAVE_ZERO_NATIVE_IPC_OK 0
#define SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT 1
#define SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED 2
#define SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED 3
#define SPACEWAVE_ZERO_NATIVE_IPC_READ_FAILED 4
#define SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE 5
#define SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR 6
#define SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE 7
#define SPACEWAVE_ZERO_NATIVE_IPC_CANCELLED 8
#define SPACEWAVE_ZERO_NATIVE_IPC_STREAM_CLOSED 9

typedef struct SpacewaveZeroNativeIpcError {
    int32_t code;
    char message[256];
} SpacewaveZeroNativeIpcError;

typedef struct SpacewaveZeroNativeIpcStream SpacewaveZeroNativeIpcStream;

typedef void (*SpacewaveZeroNativeIpcStreamPacketCallback)(
    void* user_data,
    uint32_t stream_id,
    const uint8_t* data,
    size_t data_len);

typedef void (*SpacewaveZeroNativeIpcStreamCloseCallback)(
    void* user_data,
    uint32_t stream_id,
    int32_t code,
    const char* message);

typedef struct SpacewaveZeroNativeIpcStreamCallbacks {
    void* user_data;
    SpacewaveZeroNativeIpcStreamPacketCallback on_packet;
    SpacewaveZeroNativeIpcStreamCloseCallback on_close;
} SpacewaveZeroNativeIpcStreamCallbacks;

#ifdef __cplusplus
extern "C" {
#endif

// Returns a present-state description of the available IPC transport surface.
const char* spacewave_zero_native_starpc_transport_status();

// Sends a StarPC echo request over a native pipe/socket endpoint.
//
// The endpoint is a Unix-domain socket path on Unix and a named-pipe path on
// Windows. The wire contract is intentionally small for the current transport:
//   request frame:  4-byte little-endian length + request bytes
//   response frame: 4-byte little-endian length + status byte + response bytes
// The response status byte is 0 for echo data and 1 for a remote error message.
//
// On success, response_len receives the echo response byte count. If the
// response buffer is too small, response_len receives the required size and the
// function returns SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE.
int32_t spacewave_zero_native_starpc_echo(
    const char* endpoint,
    const uint8_t* request,
    size_t request_len,
    uint8_t* response,
    size_t response_cap,
    size_t* response_len,
    SpacewaveZeroNativeIpcError* error);

// Opens a callback-driven framed stream over the native pipe/socket endpoint.
//
// The current stream probe uses a deliberately small frame shape:
//   frame: 4-byte little-endian length + type byte + stream_id LE32 + payload
// Frame types are internal to this probe. The ABI owns the native handle and
// reader thread until close/cancel is called.
int32_t spacewave_zero_native_starpc_stream_open(
    const char* endpoint,
    uint32_t stream_id,
    const SpacewaveZeroNativeIpcStreamCallbacks* callbacks,
    SpacewaveZeroNativeIpcStream** stream,
    SpacewaveZeroNativeIpcError* error);

// Sends one ordered packet on the stream.
int32_t spacewave_zero_native_starpc_stream_send(
    SpacewaveZeroNativeIpcStream* stream,
    const uint8_t* data,
    size_t data_len,
    SpacewaveZeroNativeIpcError* error);

// Closes and releases the stream. The close callback is delivered at most once.
int32_t spacewave_zero_native_starpc_stream_close(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error);

// Sends a cancel frame, closes the native handle, and releases the stream.
int32_t spacewave_zero_native_starpc_stream_cancel(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error);

// WebView IPC packet streams use the same framed packet C ABI as StarPC streams.
// These entrypoints expose the WebView bridge name without starting renderer
// backend boot.
int32_t spacewave_zero_native_webview_ipc_stream_open(
    const char* endpoint,
    uint32_t stream_id,
    const SpacewaveZeroNativeIpcStreamCallbacks* callbacks,
    SpacewaveZeroNativeIpcStream** stream,
    SpacewaveZeroNativeIpcError* error);

int32_t spacewave_zero_native_webview_ipc_stream_send(
    SpacewaveZeroNativeIpcStream* stream,
    const uint8_t* data,
    size_t data_len,
    SpacewaveZeroNativeIpcError* error);

int32_t spacewave_zero_native_webview_ipc_stream_close(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error);

int32_t spacewave_zero_native_webview_ipc_stream_cancel(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error);

#ifdef __cplusplus
}
#endif
