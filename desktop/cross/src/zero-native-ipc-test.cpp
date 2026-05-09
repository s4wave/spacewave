#include "zero-native-ipc.h"

#include <chrono>
#include <condition_variable>
#include <cerrno>
#include <cstdlib>
#include <cstring>
#include <iostream>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#ifdef _WIN32
int main() {
    std::cout << spacewave_zero_native_starpc_transport_status() << "\n";
    std::cout << "zero-native-ipc-test: Windows named-pipe coverage is not implemented\n";
    return 0;
}
#else

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

namespace {

constexpr uint8_t kResponseOK = 0;
constexpr uint8_t kResponseError = 1;
constexpr uint8_t kStreamFrameOpen = 1;
constexpr uint8_t kStreamFramePacket = 2;
constexpr uint8_t kStreamFrameClose = 3;
constexpr uint8_t kStreamFrameCancel = 4;

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

bool writeAll(int fd, const uint8_t* data, size_t len) {
    while (len > 0) {
        ssize_t n = write(fd, data, len);
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
}

bool readExact(int fd, uint8_t* data, size_t len) {
    while (len > 0) {
        ssize_t n = read(fd, data, len);
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
}

bool writeFrame(int fd, const std::vector<uint8_t>& data) {
    uint8_t lenBuf[4];
    writeLE32(lenBuf, static_cast<uint32_t>(data.size()));
    return writeAll(fd, lenBuf, sizeof(lenBuf)) &&
        (data.empty() || writeAll(fd, data.data(), data.size()));
}

bool readFrame(int fd, std::vector<uint8_t>* out) {
    uint8_t lenBuf[4];
    if (!readExact(fd, lenBuf, sizeof(lenBuf))) {
        return false;
    }
    uint32_t len = readLE32(lenBuf);
    out->resize(len);
    return len == 0 || readExact(fd, out->data(), len);
}

struct StreamFrame {
    uint8_t type = 0;
    uint32_t streamID = 0;
    std::vector<uint8_t> payload;
};

bool writeStreamFrame(int fd, uint8_t type, uint32_t streamID, const std::vector<uint8_t>& payload) {
    std::vector<uint8_t> frame;
    frame.resize(payload.size() + 5);
    frame[0] = type;
    writeLE32(frame.data() + 1, streamID);
    if (!payload.empty()) {
        std::memcpy(frame.data() + 5, payload.data(), payload.size());
    }
    return writeFrame(fd, frame);
}

bool readStreamFrame(int fd, StreamFrame* frame) {
    std::vector<uint8_t> raw;
    if (!readFrame(fd, &raw)) {
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

struct ServerResult {
    std::vector<uint8_t> request;
    bool sawCleanClose = false;
    std::string error;
};

struct StreamServerResult {
    uint32_t streamID = 0;
    std::vector<std::string> packets;
    bool sawClientClose = false;
    bool sawClientCancel = false;
    bool sawCleanClose = false;
    std::string error;
};

struct StreamClientState {
    std::mutex mutex;
    std::condition_variable changed;
    uint32_t expectedStreamID = 0;
    std::vector<std::string> packets;
    int32_t closeCode = -1;
    std::string closeMessage;
    bool callbackUserDataMatched = true;
};

std::string makeSocketPath(const char* suffix) {
    char dirTemplate[] = "/tmp/spacewave-zero-native-ipc.XXXXXX";
    char* dir = mkdtemp(dirTemplate);
    if (dir == nullptr) {
        std::cerr << "mkdtemp failed: " << std::strerror(errno) << "\n";
        std::exit(1);
    }
    return std::string(dir) + "/" + suffix + ".sock";
}

void cleanupSocketPath(const std::string& socketPath) {
    unlink(socketPath.c_str());
    size_t slash = socketPath.rfind('/');
    if (slash != std::string::npos) {
        std::string dir = socketPath.substr(0, slash);
        rmdir(dir.c_str());
    }
}

int listenUnix(const std::string& socketPath) {
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        std::cerr << "socket failed: " << std::strerror(errno) << "\n";
        std::exit(1);
    }

    sockaddr_un addr;
    std::memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    if (socketPath.size() >= sizeof(addr.sun_path)) {
        std::cerr << "socket path too long\n";
        std::exit(1);
    }
    std::memcpy(addr.sun_path, socketPath.c_str(), socketPath.size() + 1);
    if (bind(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) != 0) {
        std::cerr << "bind failed: " << std::strerror(errno) << "\n";
        std::exit(1);
    }
    if (listen(fd, 8) != 0) {
        std::cerr << "listen failed: " << std::strerror(errno) << "\n";
        std::exit(1);
    }
    return fd;
}

ServerResult runServer(int listenFd, const std::vector<uint8_t>& responsePayload, bool remoteError) {
    ServerResult result;
    int fd = accept(listenFd, nullptr, nullptr);
    if (fd < 0) {
        result.error = std::string("accept failed: ") + std::strerror(errno);
        return result;
    }

    if (!readFrame(fd, &result.request)) {
        result.error = "read request failed";
        close(fd);
        return result;
    }

    std::vector<uint8_t> response;
    response.reserve(responsePayload.size() + 1);
    response.push_back(remoteError ? kResponseError : kResponseOK);
    response.insert(response.end(), responsePayload.begin(), responsePayload.end());
    if (!writeFrame(fd, response)) {
        result.error = "write response failed";
        close(fd);
        return result;
    }

    uint8_t eofProbe = 0;
    ssize_t n = read(fd, &eofProbe, 1);
    result.sawCleanClose = n == 0;
    close(fd);
    return result;
}

bool contains(const char* haystack, const char* needle) {
    return std::strstr(haystack, needle) != nullptr;
}

std::string bytesToString(const std::vector<uint8_t>& bytes) {
    return std::string(reinterpret_cast<const char*>(bytes.data()), bytes.size());
}

std::vector<uint8_t> stringBytes(const char* value) {
    return std::vector<uint8_t>(value, value + std::strlen(value));
}

void require(bool ok, const char* message) {
    if (!ok) {
        std::cerr << "require failed: " << message << "\n";
        std::exit(1);
    }
}

void onStreamPacket(void* userData, uint32_t streamID, const uint8_t* data, size_t dataLen) {
    StreamClientState* state = static_cast<StreamClientState*>(userData);
    std::lock_guard<std::mutex> lock(state->mutex);
    state->callbackUserDataMatched = state->callbackUserDataMatched &&
        streamID == state->expectedStreamID;
    if (dataLen == 0) {
        state->packets.emplace_back();
    } else {
        state->packets.emplace_back(reinterpret_cast<const char*>(data), dataLen);
    }
    state->changed.notify_all();
}

void onStreamClose(void* userData, uint32_t streamID, int32_t code, const char* message) {
    StreamClientState* state = static_cast<StreamClientState*>(userData);
    std::lock_guard<std::mutex> lock(state->mutex);
    state->callbackUserDataMatched = state->callbackUserDataMatched &&
        streamID == state->expectedStreamID;
    state->closeCode = code;
    state->closeMessage = message == nullptr ? "" : message;
    state->changed.notify_all();
}

bool waitForStreamState(StreamClientState* state, size_t packetCount, bool closed) {
    std::unique_lock<std::mutex> lock(state->mutex);
    return state->changed.wait_for(lock, std::chrono::seconds(5), [&]() {
        bool packetReady = state->packets.size() >= packetCount;
        bool closeReady = !closed || state->closeCode != -1;
        return packetReady && closeReady;
    });
}

StreamServerResult runPacketStreamServer(int listenFd, size_t expectedPackets) {
    StreamServerResult result;
    int fd = accept(listenFd, nullptr, nullptr);
    if (fd < 0) {
        result.error = std::string("accept failed: ") + std::strerror(errno);
        return result;
    }

    StreamFrame frame;
    if (!readStreamFrame(fd, &frame)) {
        result.error = "read stream open failed";
        close(fd);
        return result;
    }
    if (frame.type != kStreamFrameOpen) {
        result.error = "first stream frame was not open";
        close(fd);
        return result;
    }
    result.streamID = frame.streamID;

    for (size_t i = 0; i < expectedPackets; ++i) {
        if (!readStreamFrame(fd, &frame)) {
            result.error = "read stream packet failed";
            close(fd);
            return result;
        }
        if (frame.type != kStreamFramePacket || frame.streamID != result.streamID) {
            result.error = "unexpected stream packet frame";
            close(fd);
            return result;
        }
        result.packets.push_back(bytesToString(frame.payload));
        if (!writeStreamFrame(fd, kStreamFramePacket, result.streamID, frame.payload)) {
            result.error = "write stream packet failed";
            close(fd);
            return result;
        }
    }

    std::string closeMessage = "server-close-" + std::to_string(result.streamID);
    if (!writeStreamFrame(fd, kStreamFrameClose, result.streamID, stringBytes(closeMessage.c_str()))) {
        result.error = "write stream close failed";
        close(fd);
        return result;
    }

    if (readStreamFrame(fd, &frame)) {
        if (frame.type == kStreamFrameClose && frame.streamID == result.streamID) {
            result.sawClientClose = true;
        }
    } else {
        result.sawCleanClose = true;
    }
    close(fd);
    return result;
}

StreamServerResult runCancelStreamServer(int listenFd) {
    StreamServerResult result;
    int fd = accept(listenFd, nullptr, nullptr);
    if (fd < 0) {
        result.error = std::string("accept failed: ") + std::strerror(errno);
        return result;
    }

    StreamFrame frame;
    if (!readStreamFrame(fd, &frame)) {
        result.error = "read cancel stream open failed";
        close(fd);
        return result;
    }
    if (frame.type != kStreamFrameOpen) {
        result.error = "first cancel stream frame was not open";
        close(fd);
        return result;
    }
    result.streamID = frame.streamID;

    for (;;) {
        if (!readStreamFrame(fd, &frame)) {
            result.error = "cancel stream closed before cancel frame";
            close(fd);
            return result;
        }
        if (frame.streamID != result.streamID) {
            result.error = "cancel stream id mismatch";
            close(fd);
            return result;
        }
        if (frame.type == kStreamFramePacket) {
            result.packets.push_back(bytesToString(frame.payload));
            continue;
        }
        if (frame.type == kStreamFrameCancel) {
            result.sawClientCancel = true;
            break;
        }
        result.error = "unexpected cancel stream frame";
        close(fd);
        return result;
    }

    uint8_t eofProbe = 0;
    ssize_t n = read(fd, &eofProbe, 1);
    result.sawCleanClose = n == 0;
    close(fd);
    return result;
}

void runEchoCase() {
    std::string socketPath = makeSocketPath("echo");
    int listenFd = listenUnix(socketPath);
    std::vector<uint8_t> payload = {'q', 'u', 'o', 'r', 'r', 'a'};
    ServerResult serverResult;
    std::thread server([&]() {
        serverResult = runServer(listenFd, payload, false);
    });

    uint8_t response[64];
    size_t responseLen = 0;
    SpacewaveZeroNativeIpcError error;
    int32_t code = spacewave_zero_native_starpc_echo(
        socketPath.c_str(),
        payload.data(),
        payload.size(),
        response,
        sizeof(response),
        &responseLen,
        &error);
    server.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "echo code ok");
    require(responseLen == payload.size(), "echo response length");
    require(std::memcmp(response, payload.data(), payload.size()) == 0, "echo response bytes");
    require(serverResult.request == payload, "server received request bytes");
    require(serverResult.sawCleanClose, "server observed EOF after client close");
    require(serverResult.error.empty(), "server finished without error");
}

void runRemoteErrorCase() {
    std::string socketPath = makeSocketPath("remote-error");
    int listenFd = listenUnix(socketPath);
    std::string msg = "remote echo failed";
    std::vector<uint8_t> remoteError(msg.begin(), msg.end());
    ServerResult serverResult;
    std::thread server([&]() {
        serverResult = runServer(listenFd, remoteError, true);
    });

    uint8_t response[64];
    size_t responseLen = 0;
    SpacewaveZeroNativeIpcError error;
    const uint8_t request[] = {'f', 'a', 'i', 'l'};
    int32_t code = spacewave_zero_native_starpc_echo(
        socketPath.c_str(),
        request,
        sizeof(request),
        response,
        sizeof(response),
        &responseLen,
        &error);
    server.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(code == SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR, "remote error code");
    require(error.code == SPACEWAVE_ZERO_NATIVE_IPC_REMOTE_ERROR, "remote error struct code");
    require(contains(error.message, msg.c_str()), "remote error message propagated");
    require(responseLen == 0, "remote error has no response payload");
    require(serverResult.sawCleanClose, "remote error path clean close");
    require(serverResult.error.empty(), "remote error server finished without error");
}

void runTooLargeCase() {
    std::string socketPath = makeSocketPath("too-large");
    int listenFd = listenUnix(socketPath);
    std::vector<uint8_t> payload = {'l', 'a', 'r', 'g', 'e'};
    ServerResult serverResult;
    std::thread server([&]() {
        serverResult = runServer(listenFd, payload, false);
    });

    uint8_t response[2];
    size_t responseLen = 0;
    SpacewaveZeroNativeIpcError error;
    const uint8_t request[] = {'s', 'm', 'a', 'l', 'l'};
    int32_t code = spacewave_zero_native_starpc_echo(
        socketPath.c_str(),
        request,
        sizeof(request),
        response,
        sizeof(response),
        &responseLen,
        &error);
    server.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(code == SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE, "too-large code");
    require(responseLen == payload.size(), "too-large reports required length");
    require(error.code == SPACEWAVE_ZERO_NATIVE_IPC_RESPONSE_TOO_LARGE, "too-large error struct code");
    require(serverResult.sawCleanClose, "too-large path clean close");
    require(serverResult.error.empty(), "too-large server finished without error");
}

void runConnectFailureCase() {
    std::string socketPath = makeSocketPath("missing");
    uint8_t response[8];
    size_t responseLen = 0;
    SpacewaveZeroNativeIpcError error;
    const uint8_t request[] = {'x'};
    int32_t code = spacewave_zero_native_starpc_echo(
        socketPath.c_str(),
        request,
        sizeof(request),
        response,
        sizeof(response),
        &responseLen,
        &error);
    cleanupSocketPath(socketPath);
    require(code == SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "connect failure code");
    require(error.code == SPACEWAVE_ZERO_NATIVE_IPC_CONNECT_FAILED, "connect failure error struct code");
    require(responseLen == 0, "connect failure has no response");
}

void runConcurrentStreamCase() {
    std::string socketPath = makeSocketPath("streams");
    int listenFd = listenUnix(socketPath);
    StreamServerResult serverA;
    StreamServerResult serverB;
    std::thread acceptA([&]() {
        serverA = runPacketStreamServer(listenFd, 3);
    });
    std::thread acceptB([&]() {
        serverB = runPacketStreamServer(listenFd, 3);
    });

    StreamClientState stateA;
    StreamClientState stateB;
    stateA.expectedStreamID = 101;
    stateB.expectedStreamID = 202;
    SpacewaveZeroNativeIpcStreamCallbacks callbacksA = {&stateA, onStreamPacket, onStreamClose};
    SpacewaveZeroNativeIpcStreamCallbacks callbacksB = {&stateB, onStreamPacket, onStreamClose};
    SpacewaveZeroNativeIpcStream* streamA = nullptr;
    SpacewaveZeroNativeIpcStream* streamB = nullptr;
    SpacewaveZeroNativeIpcError error;

    int32_t codeA = spacewave_zero_native_starpc_stream_open(
        socketPath.c_str(),
        stateA.expectedStreamID,
        &callbacksA,
        &streamA,
        &error);
    int32_t codeB = spacewave_zero_native_starpc_stream_open(
        socketPath.c_str(),
        stateB.expectedStreamID,
        &callbacksB,
        &streamB,
        &error);
    require(codeA == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream A open");
    require(codeB == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream B open");

    std::thread sendA([&]() {
        const char* packets[] = {"a0", "a1", "a2"};
        SpacewaveZeroNativeIpcError sendError;
        for (const char* packet : packets) {
            std::vector<uint8_t> bytes = stringBytes(packet);
            int32_t code = spacewave_zero_native_starpc_stream_send(
                streamA,
                bytes.data(),
                bytes.size(),
                &sendError);
            require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream A send");
        }
    });
    std::thread sendB([&]() {
        const char* packets[] = {"b0", "b1", "b2"};
        SpacewaveZeroNativeIpcError sendError;
        for (const char* packet : packets) {
            std::vector<uint8_t> bytes = stringBytes(packet);
            int32_t code = spacewave_zero_native_starpc_stream_send(
                streamB,
                bytes.data(),
                bytes.size(),
                &sendError);
            require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream B send");
        }
    });
    sendA.join();
    sendB.join();

    require(waitForStreamState(&stateA, 3, true), "stream A callbacks completed");
    require(waitForStreamState(&stateB, 3, true), "stream B callbacks completed");
    codeA = spacewave_zero_native_starpc_stream_close(streamA, &error);
    codeB = spacewave_zero_native_starpc_stream_close(streamB, &error);
    acceptA.join();
    acceptB.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(codeA == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream A close");
    require(codeB == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream B close");
    require(stateA.callbackUserDataMatched, "stream A callback ownership");
    require(stateB.callbackUserDataMatched, "stream B callback ownership");
    require(stateA.closeCode == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream A close callback code");
    require(stateB.closeCode == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream B close callback code");
    require(stateA.closeMessage == "server-close-101", "stream A close message");
    require(stateB.closeMessage == "server-close-202", "stream B close message");
    require((stateA.packets == std::vector<std::string>{"a0", "a1", "a2"}), "stream A packet order");
    require((stateB.packets == std::vector<std::string>{"b0", "b1", "b2"}), "stream B packet order");
    require(serverA.error.empty(), "concurrent stream server A finished without error");
    require(serverB.error.empty(), "concurrent stream server B finished without error");
}

void runCancelStreamCase() {
    std::string socketPath = makeSocketPath("cancel-stream");
    int listenFd = listenUnix(socketPath);
    StreamServerResult serverResult;
    std::thread server([&]() {
        serverResult = runCancelStreamServer(listenFd);
    });

    StreamClientState state;
    state.expectedStreamID = 303;
    SpacewaveZeroNativeIpcStreamCallbacks callbacks = {&state, onStreamPacket, onStreamClose};
    SpacewaveZeroNativeIpcStream* stream = nullptr;
    SpacewaveZeroNativeIpcError error;
    int32_t code = spacewave_zero_native_starpc_stream_open(
        socketPath.c_str(),
        state.expectedStreamID,
        &callbacks,
        &stream,
        &error);
    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "cancel stream open");

    std::vector<uint8_t> packet = stringBytes("before-cancel");
    code = spacewave_zero_native_starpc_stream_send(stream, packet.data(), packet.size(), &error);
    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "cancel stream send");
    code = spacewave_zero_native_starpc_stream_cancel(stream, &error);
    server.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "stream cancel");
    require(state.callbackUserDataMatched, "cancel callback ownership");
    require(state.closeCode == SPACEWAVE_ZERO_NATIVE_IPC_CANCELLED, "cancel close callback code");
    require(state.closeMessage == "stream cancelled", "cancel close callback message");
    require(serverResult.error.empty(), "cancel stream server finished without error");
    require(serverResult.streamID == state.expectedStreamID, "cancel stream id");
    require((serverResult.packets == std::vector<std::string>{"before-cancel"}), "cancel packet before cancel");
    require(serverResult.sawClientCancel, "server observed cancel frame");
    require(serverResult.sawCleanClose, "server observed clean close after cancel");
}

void runWebViewIpcBridgeFrameCase() {
    std::string socketPath = makeSocketPath("webview-ipc");
    int listenFd = listenUnix(socketPath);
    StreamServerResult serverResult;
    std::thread server([&]() {
        serverResult = runPacketStreamServer(listenFd, 1);
    });

    StreamClientState state;
    state.expectedStreamID = 404;
    SpacewaveZeroNativeIpcStreamCallbacks callbacks = {&state, onStreamPacket, onStreamClose};
    SpacewaveZeroNativeIpcStream* stream = nullptr;
    SpacewaveZeroNativeIpcError error;
    int32_t code = spacewave_zero_native_webview_ipc_stream_open(
        socketPath.c_str(),
        state.expectedStreamID,
        &callbacks,
        &stream,
        &error);
    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "webview ipc stream open");

    std::vector<uint8_t> rpcFrame = {
        0x1a, 0x0c, 's', 't', 'a', 'r', 'p', 'c', '-', 'f',
        'r',  'a',  'm', 'e',
    };
    code = spacewave_zero_native_webview_ipc_stream_send(stream, rpcFrame.data(), rpcFrame.size(), &error);
    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "webview ipc stream send");
    require(waitForStreamState(&state, 1, true), "webview ipc callbacks completed");
    code = spacewave_zero_native_webview_ipc_stream_close(stream, &error);
    server.join();
    close(listenFd);
    cleanupSocketPath(socketPath);

    require(code == SPACEWAVE_ZERO_NATIVE_IPC_OK, "webview ipc stream close");
    require(state.callbackUserDataMatched, "webview ipc callback ownership");
    require(state.closeCode == SPACEWAVE_ZERO_NATIVE_IPC_OK, "webview ipc close callback code");
    require(state.closeMessage == "server-close-404", "webview ipc close message");
    require((state.packets == std::vector<std::string>{bytesToString(rpcFrame)}), "webview ipc packet echo");
    require(serverResult.error.empty(), "webview ipc server finished without error");
    require(serverResult.streamID == state.expectedStreamID, "webview ipc stream id");
    require((serverResult.packets == std::vector<std::string>{bytesToString(rpcFrame)}), "webview ipc server packet bytes");
}

}  // namespace

int main() {
    const char* status = spacewave_zero_native_starpc_transport_status();
    std::cout << status << "\n";
    require(contains(status, "StarPC IPC transport"), "status exposes starpc ipc transport");
    require(contains(status, "WebView IPC packet-stream bridge"), "status exposes webview ipc bridge");
    require(contains(status, "Resource SDK"), "status exposes Resource SDK backend invariant");
    require(contains(status, "no renderer-local transport fallback"), "status rejects renderer-local fallback");
    require(contains(status, "renderer backend boot"), "status keeps backend boot out of probe scope");
    runEchoCase();
    runRemoteErrorCase();
    runTooLargeCase();
    runConnectFailureCase();
    runConcurrentStreamCase();
    runCancelStreamCase();
    runWebViewIpcBridgeFrameCase();
    std::cout << "zero-native-ipc-test: ok\n";
    return 0;
}

#endif
