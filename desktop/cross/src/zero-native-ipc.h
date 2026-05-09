#pragma once

#include <stddef.h>
#include <stdint.h>

// Native IPC boundary for zero-native WebRuntime transport probes.
//
// This proves a zero-native-owned C ABI can perform an echo round trip over a
// native pipe/socket with explicit remote errors and clean close behavior. It is
// not a renderer boot path, WebView bridge, package hook, or full WebRuntime
// transport.

#define SPACEWAVE_ZERO_NATIVE_IPC_OK 0
#define SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT 1
#define SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED 2
#define SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED 3
#define SPACEWAVE_ZERO_NATIVE_IPC_READ_FAILED 4
#define SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE 5
#define SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR 6
#define SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE 7

typedef struct SpacewaveZeroNativeIpcError {
    int32_t code;
    char message[256];
} SpacewaveZeroNativeIpcError;

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

#ifdef __cplusplus
}
#endif
