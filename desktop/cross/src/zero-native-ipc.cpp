#include "zero-native-ipc.h"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

#ifdef _WIN32
#include <windows.h>
#else
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#endif

namespace {

constexpr uint32_t kMaxEchoFrameSize = 1024 * 1024;
constexpr uint8_t kResponseOK = 0;
constexpr uint8_t kResponseError = 1;

#ifdef _WIN32
using NativeHandle = HANDLE;
constexpr NativeHandle kInvalidHandle = INVALID_HANDLE_VALUE;
#else
using NativeHandle = int;
constexpr NativeHandle kInvalidHandle = -1;
#endif

void setError(SpacewaveZeroNativeIpcError* error, int32_t code, const char* message) {
    if (error == nullptr) {
        return;
    }
    error->code = code;
    std::snprintf(error->message, sizeof(error->message), "%s", message == nullptr ? "" : message);
}

void setSystemError(
    SpacewaveZeroNativeIpcError* error,
    int32_t code,
    const char* action) {
#ifdef _WIN32
    DWORD err = GetLastError();
    char msg[256];
    std::snprintf(msg, sizeof(msg), "%s: windows error %lu", action, static_cast<unsigned long>(err));
    setError(error, code, msg);
#else
    char msg[256];
    std::snprintf(msg, sizeof(msg), "%s: %s", action, std::strerror(errno));
    setError(error, code, msg);
#endif
}

void clearError(SpacewaveZeroNativeIpcError* error) {
    setError(error, SPACEWAVE_ZERO_NATIVE_IPC_OK, "");
}

void closeHandle(NativeHandle handle) {
    if (handle == kInvalidHandle) {
        return;
    }
#ifdef _WIN32
    FlushFileBuffers(handle);
    CloseHandle(handle);
#else
    shutdown(handle, SHUT_RDWR);
    close(handle);
#endif
}

NativeHandle connectEndpoint(const char* endpoint, SpacewaveZeroNativeIpcError* error) {
#ifdef _WIN32
    NativeHandle pipe = CreateFileA(
        endpoint,
        GENERIC_READ | GENERIC_WRITE,
        0,
        nullptr,
        OPEN_EXISTING,
        0,
        nullptr);
    if (pipe == INVALID_HANDLE_VALUE) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "connect named pipe");
        return kInvalidHandle;
    }
    return pipe;
#else
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "create unix socket");
        return kInvalidHandle;
    }

#ifdef SO_NOSIGPIPE
    int set = 1;
    (void)setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &set, sizeof(set));
#endif

    sockaddr_un addr;
    std::memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    size_t endpointLen = std::strlen(endpoint);
    if (endpointLen >= sizeof(addr.sun_path)) {
        closeHandle(fd);
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "unix socket path is too long");
        return kInvalidHandle;
    }
    std::memcpy(addr.sun_path, endpoint, endpointLen + 1);

    if (connect(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) != 0) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "connect unix socket");
        closeHandle(fd);
        return kInvalidHandle;
    }
    return fd;
#endif
}

bool writeAll(NativeHandle handle, const uint8_t* data, size_t len) {
#ifdef _WIN32
    HANDLE pipe = handle;
    while (len > 0) {
        DWORD chunk = len > 0x7ffff000u ? 0x7ffff000u : static_cast<DWORD>(len);
        DWORD written = 0;
        if (!WriteFile(pipe, data, chunk, &written, nullptr)) {
            return false;
        }
        if (written == 0) {
            return false;
        }
        data += written;
        len -= written;
    }
    return true;
#else
    while (len > 0) {
#ifdef MSG_NOSIGNAL
        ssize_t n = send(handle, data, len, MSG_NOSIGNAL);
#else
        ssize_t n = send(handle, data, len, 0);
#endif
        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n <= 0) {
            return false;
        }
        data += static_cast<size_t>(n);
        len -= static_cast<size_t>(n);
    }
    return true;
#endif
}

bool readExact(NativeHandle handle, uint8_t* data, size_t len) {
#ifdef _WIN32
    HANDLE pipe = handle;
    while (len > 0) {
        DWORD chunk = len > 0x7ffff000u ? 0x7ffff000u : static_cast<DWORD>(len);
        DWORD nread = 0;
        if (!ReadFile(pipe, data, chunk, &nread, nullptr)) {
            return false;
        }
        if (nread == 0) {
            return false;
        }
        data += nread;
        len -= nread;
    }
    return true;
#else
    while (len > 0) {
        ssize_t n = read(handle, data, len);
        if (n < 0 && errno == EINTR) {
            continue;
        }
        if (n <= 0) {
            return false;
        }
        data += static_cast<size_t>(n);
        len -= static_cast<size_t>(n);
    }
    return true;
#endif
}

void writeLE32(uint8_t* out, uint32_t value) {
    out[0] = static_cast<uint8_t>(value);
    out[1] = static_cast<uint8_t>(value >> 8);
    out[2] = static_cast<uint8_t>(value >> 16);
    out[3] = static_cast<uint8_t>(value >> 24);
}

uint32_t readLE32(const uint8_t* in) {
    return static_cast<uint32_t>(in[0]) |
        (static_cast<uint32_t>(in[1]) << 8) |
        (static_cast<uint32_t>(in[2]) << 16) |
        (static_cast<uint32_t>(in[3]) << 24);
}

bool writeFrame(NativeHandle handle, const uint8_t* data, size_t len) {
    if (len > kMaxEchoFrameSize) {
        return false;
    }
    uint8_t lenBuf[4];
    writeLE32(lenBuf, static_cast<uint32_t>(len));
    if (!writeAll(handle, lenBuf, sizeof(lenBuf))) {
        return false;
    }
    return len == 0 || writeAll(handle, data, len);
}

bool readFrame(NativeHandle handle, std::vector<uint8_t>* out) {
    uint8_t lenBuf[4];
    if (!readExact(handle, lenBuf, sizeof(lenBuf))) {
        return false;
    }
    uint32_t len = readLE32(lenBuf);
    if (len > kMaxEchoFrameSize) {
        return false;
    }
    out->resize(len);
    return len == 0 || readExact(handle, out->data(), len);
}

}  // namespace

extern "C" const char* spacewave_zero_native_starpc_transport_status() {
    return "zero-native StarPC IPC transport: native socket echo only; renderer, WebView bridge, package, and e2e integration are not implemented by this transport probe";
}

extern "C" int32_t spacewave_zero_native_starpc_echo(
    const char* endpoint,
    const uint8_t* request,
    size_t request_len,
    uint8_t* response,
    size_t response_cap,
    size_t* response_len,
    SpacewaveZeroNativeIpcError* error) {
    clearError(error);
    if (response_len != nullptr) {
        *response_len = 0;
    }

    if (endpoint == nullptr || endpoint[0] == '\0' || response_len == nullptr ||
        (request_len != 0 && request == nullptr) ||
        (response_cap != 0 && response == nullptr)) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "invalid echo arguments");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }
    if (request_len > kMaxEchoFrameSize) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "request frame exceeds echo limit");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }

    NativeHandle handle = connectEndpoint(endpoint, error);
    if (handle == kInvalidHandle) {
        return SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED;
    }

    int32_t result = SPACEWAVE_ZERO_NATIVE_IPC_OK;
    if (!writeFrame(handle, request, request_len)) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED, "write echo request");
        result = SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED;
    } else {
        std::vector<uint8_t> frame;
        if (!readFrame(handle, &frame)) {
            setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_READ_FAILED, "read echo response");
            result = SPACEWAVE_ZERO_NATIVE_IPC_READ_FAILED;
        } else if (frame.empty()) {
            setError(error, SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE, "empty echo response frame");
            result = SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE;
        } else if (frame[0] == kResponseError) {
            std::string remoteError(reinterpret_cast<const char*>(frame.data() + 1), frame.size() - 1);
            setError(error, SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR, remoteError.c_str());
            result = SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR;
        } else if (frame[0] != kResponseOK) {
            setError(error, SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE, "unknown echo response status");
            result = SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE;
        } else {
            size_t payloadLen = frame.size() - 1;
            *response_len = payloadLen;
            if (payloadLen > response_cap) {
                setError(error, SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE, "echo response buffer too small");
                result = SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE;
            } else if (payloadLen != 0) {
                std::memcpy(response, frame.data() + 1, payloadLen);
            }
        }
    }

    closeHandle(handle);
    return result;
}
