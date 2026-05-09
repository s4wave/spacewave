#include "zero-native-ipc.h"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#ifdef _WIN32
#include <windows.h>
#else
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#endif

#ifdef _WIN32
using NativeHandle = HANDLE;
const NativeHandle kInvalidHandle = INVALID_HANDLE_VALUE;
#else
using NativeHandle = int;
constexpr NativeHandle kInvalidHandle = -1;
#endif

struct SpacewaveZeroNativeIpcStream {
    NativeHandle handle = kInvalidHandle;
    uint32_t streamID = 0;
    SpacewaveZeroNativeIpcStreamCallbacks callbacks;
    std::mutex mutex;
    std::thread reader;
    bool closeDelivered = false;
};

namespace {

constexpr uint32_t kMaxEchoFrameSize = 1024 * 1024;
constexpr uint32_t kMaxStreamFrameSize = 1024 * 1024;
constexpr uint8_t kResponseOK = 0;
constexpr uint8_t kResponseError = 1;
constexpr uint8_t kStreamFrameOpen = 1;
constexpr uint8_t kStreamFramePacket = 2;
constexpr uint8_t kStreamFrameClose = 3;
constexpr uint8_t kStreamFrameCancel = 4;
constexpr uint8_t kStreamFrameError = 5;

struct StreamFrame {
    uint8_t type = 0;
    uint32_t streamID = 0;
    std::vector<uint8_t> payload;
};

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

bool writeStreamFrame(
    NativeHandle handle,
    uint8_t type,
    uint32_t streamID,
    const uint8_t* data,
    size_t len) {
    if (len > kMaxStreamFrameSize - 5) {
        return false;
    }
    std::vector<uint8_t> frame;
    frame.resize(len + 5);
    frame[0] = type;
    writeLE32(frame.data() + 1, streamID);
    if (len != 0) {
        std::memcpy(frame.data() + 5, data, len);
    }
    return writeFrame(handle, frame.data(), frame.size());
}

bool readStreamFrame(NativeHandle handle, StreamFrame* frame) {
    std::vector<uint8_t> raw;
    if (!readFrame(handle, &raw)) {
        return false;
    }
    if (raw.size() < 5) {
        return false;
    }
    frame->type = raw[0];
    frame->streamID = readLE32(raw.data() + 1);
    frame->payload.assign(raw.begin() + 5, raw.end());
    return true;
}

std::string payloadString(const std::vector<uint8_t>& payload) {
    return std::string(reinterpret_cast<const char*>(payload.data()), payload.size());
}

bool markCloseDelivered(SpacewaveZeroNativeIpcStream* stream) {
    std::lock_guard<std::mutex> lock(stream->mutex);
    if (stream->closeDelivered) {
        return false;
    }
    stream->closeDelivered = true;
    return true;
}

void deliverClose(SpacewaveZeroNativeIpcStream* stream, int32_t code, const char* message) {
    if (!markCloseDelivered(stream)) {
        return;
    }
    if (stream->callbacks.on_close != nullptr) {
        stream->callbacks.on_close(stream->callbacks.user_data, stream->streamID, code, message);
    }
}

NativeHandle takeHandle(SpacewaveZeroNativeIpcStream* stream) {
    std::lock_guard<std::mutex> lock(stream->mutex);
    NativeHandle handle = stream->handle;
    stream->handle = kInvalidHandle;
    return handle;
}

void closeStreamHandle(SpacewaveZeroNativeIpcStream* stream) {
    NativeHandle handle = takeHandle(stream);
    if (handle != kInvalidHandle) {
        closeHandle(handle);
    }
}

void joinReader(SpacewaveZeroNativeIpcStream* stream) {
    if (!stream->reader.joinable()) {
        return;
    }
    if (stream->reader.get_id() == std::this_thread::get_id()) {
        stream->reader.detach();
        return;
    }
    stream->reader.join();
}

void streamReaderLoop(SpacewaveZeroNativeIpcStream* stream) {
    for (;;) {
        NativeHandle handle = kInvalidHandle;
        {
            std::lock_guard<std::mutex> lock(stream->mutex);
            handle = stream->handle;
        }
        if (handle == kInvalidHandle) {
            return;
        }

        StreamFrame frame;
        if (!readStreamFrame(handle, &frame)) {
            deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_READ_FAILED, "read stream frame");
            closeStreamHandle(stream);
            return;
        }
        if (frame.streamID != stream->streamID) {
            deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE, "stream id mismatch");
            closeStreamHandle(stream);
            return;
        }

        if (frame.type == kStreamFramePacket) {
            if (stream->callbacks.on_packet != nullptr) {
                const uint8_t* data = frame.payload.empty() ? nullptr : frame.payload.data();
                stream->callbacks.on_packet(
                    stream->callbacks.user_data,
                    stream->streamID,
                    data,
                    frame.payload.size());
            }
            continue;
        }

        if (frame.type == kStreamFrameClose) {
            std::string message = payloadString(frame.payload);
            deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_OK, message.c_str());
            closeStreamHandle(stream);
            return;
        }

        if (frame.type == kStreamFrameError) {
            std::string message = payloadString(frame.payload);
            deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR, message.c_str());
            closeStreamHandle(stream);
            return;
        }

        deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_BAD_RESPONSE, "unknown stream frame type");
        closeStreamHandle(stream);
        return;
    }
}

}  // namespace

extern "C" const char* spacewave_zero_native_starpc_transport_status() {
    return "zero-native StarPC IPC transport: native socket echo, callback streams, and WebView IPC packet-stream bridge; backend Resource SDK traffic must remain StarPC-over-native-IPC with no renderer-local transport fallback; renderer backend boot, package, and e2e integration are not implemented by this transport probe";
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

extern "C" int32_t spacewave_zero_native_starpc_stream_open(
    const char* endpoint,
    uint32_t stream_id,
    const SpacewaveZeroNativeIpcStreamCallbacks* callbacks,
    SpacewaveZeroNativeIpcStream** stream,
    SpacewaveZeroNativeIpcError* error) {
    clearError(error);
    if (stream != nullptr) {
        *stream = nullptr;
    }
    if (endpoint == nullptr || endpoint[0] == '\0' || callbacks == nullptr || stream == nullptr ||
        callbacks->on_close == nullptr) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "invalid stream open arguments");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }

    NativeHandle handle = connectEndpoint(endpoint, error);
    if (handle == kInvalidHandle) {
        return SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED;
    }

    if (!writeStreamFrame(handle, kStreamFrameOpen, stream_id, nullptr, 0)) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED, "write stream open");
        closeHandle(handle);
        return SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED;
    }

    SpacewaveZeroNativeIpcStream* opened = new SpacewaveZeroNativeIpcStream();
    opened->handle = handle;
    opened->streamID = stream_id;
    opened->callbacks = *callbacks;
    opened->reader = std::thread(streamReaderLoop, opened);
    *stream = opened;
    return SPACEWAVE_ZERO_NATIVE_IPC_OK;
}

extern "C" int32_t spacewave_zero_native_starpc_stream_send(
    SpacewaveZeroNativeIpcStream* stream,
    const uint8_t* data,
    size_t data_len,
    SpacewaveZeroNativeIpcError* error) {
    clearError(error);
    if (stream == nullptr || (data_len != 0 && data == nullptr)) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "invalid stream send arguments");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }
    if (data_len > kMaxStreamFrameSize - 5) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "stream packet exceeds limit");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }

    std::lock_guard<std::mutex> lock(stream->mutex);
    if (stream->handle == kInvalidHandle || stream->closeDelivered) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_STREAM_CLOSED, "stream is closed");
        return SPACEWAVE_ZERO_NATIVE_IPC_STREAM_CLOSED;
    }
    if (!writeStreamFrame(stream->handle, kStreamFramePacket, stream->streamID, data, data_len)) {
        setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED, "write stream packet");
        return SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED;
    }
    return SPACEWAVE_ZERO_NATIVE_IPC_OK;
}

extern "C" int32_t spacewave_zero_native_starpc_stream_close(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error) {
    clearError(error);
    if (stream == nullptr) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "invalid stream close arguments");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }

    bool writeFailed = false;
    {
        std::lock_guard<std::mutex> lock(stream->mutex);
        if (stream->handle != kInvalidHandle && !stream->closeDelivered) {
            if (!writeStreamFrame(stream->handle, kStreamFrameClose, stream->streamID, nullptr, 0)) {
                setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED, "write stream close");
                writeFailed = true;
            }
        }
    }
    deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_OK, "");
    closeStreamHandle(stream);
    joinReader(stream);
    delete stream;
    return writeFailed ? SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED : SPACEWAVE_ZERO_NATIVE_IPC_OK;
}

extern "C" int32_t spacewave_zero_native_starpc_stream_cancel(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error) {
    clearError(error);
    if (stream == nullptr) {
        setError(error, SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT, "invalid stream cancel arguments");
        return SPACEWAVE_ZERO_NATIVE_IPC_INVALID_ARGUMENT;
    }

    bool writeFailed = false;
    {
        std::lock_guard<std::mutex> lock(stream->mutex);
        if (stream->handle != kInvalidHandle && !stream->closeDelivered) {
            if (!writeStreamFrame(stream->handle, kStreamFrameCancel, stream->streamID, nullptr, 0)) {
                setSystemError(error, SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED, "write stream cancel");
                writeFailed = true;
            }
        }
    }
    deliverClose(stream, SPACEWAVE_ZERO_NATIVE_IPC_CANCELLED, "stream cancelled");
    closeStreamHandle(stream);
    joinReader(stream);
    delete stream;
    return writeFailed ? SPACEWAVE_ZERO_NATIVE_IPC_WRITE_FAILED : SPACEWAVE_ZERO_NATIVE_IPC_OK;
}

extern "C" int32_t spacewave_zero_native_webview_ipc_stream_open(
    const char* endpoint,
    uint32_t stream_id,
    const SpacewaveZeroNativeIpcStreamCallbacks* callbacks,
    SpacewaveZeroNativeIpcStream** stream,
    SpacewaveZeroNativeIpcError* error) {
    return spacewave_zero_native_starpc_stream_open(endpoint, stream_id, callbacks, stream, error);
}

extern "C" int32_t spacewave_zero_native_webview_ipc_stream_send(
    SpacewaveZeroNativeIpcStream* stream,
    const uint8_t* data,
    size_t data_len,
    SpacewaveZeroNativeIpcError* error) {
    return spacewave_zero_native_starpc_stream_send(stream, data, data_len, error);
}

extern "C" int32_t spacewave_zero_native_webview_ipc_stream_close(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error) {
    return spacewave_zero_native_starpc_stream_close(stream, error);
}

extern "C" int32_t spacewave_zero_native_webview_ipc_stream_cancel(
    SpacewaveZeroNativeIpcStream* stream,
    SpacewaveZeroNativeIpcError* error) {
    return spacewave_zero_native_starpc_stream_cancel(stream, error);
}
