#include "pipe-client.h"
#include <cstring>

#ifdef _WIN32
#include <windows.h>
#else
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#endif

PipeClient::PipeClient(const std::string& pipePath)
    : pipePath_(pipePath) {}

PipeClient::~PipeClient() {
    close();
}

bool PipeClient::connect() {
#ifdef _WIN32
    // Windows: connect to named pipe.
    HANDLE hPipe = CreateFileA(
        pipePath_.c_str(),
        GENERIC_READ | GENERIC_WRITE,
        0, nullptr, OPEN_EXISTING, 0, nullptr);
    if (hPipe == INVALID_HANDLE_VALUE) return false;
    fd_ = (int)(intptr_t)hPipe;
    return true;
#else
    // Unix: connect to domain socket.
    fd_ = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd_ < 0) return false;

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    if (pipePath_.size() >= sizeof(addr.sun_path)) {
        ::close(fd_);
        fd_ = -1;
        return false;
    }
    strncpy(addr.sun_path, pipePath_.c_str(), sizeof(addr.sun_path) - 1);

    if (::connect(fd_, (struct sockaddr*)&addr, sizeof(addr)) != 0) {
        ::close(fd_);
        fd_ = -1;
        return false;
    }
    return true;
#endif
}

void PipeClient::close() {
    if (readThread_.joinable()) {
        readThread_.detach();
    }
    if (fd_ >= 0) {
#ifdef _WIN32
        CloseHandle((HANDLE)(intptr_t)fd_);
#else
        ::close(fd_);
#endif
        fd_ = -1;
    }
}

bool PipeClient::sendEvent(const HelperEvent& event) {
    auto data = serializeHelperEvent(event);
    return writeFrame(data);
}

bool PipeClient::readMessage(HelperMessage& msg) {
    std::vector<uint8_t> data;
    if (!readFrame(data)) return false;
    msg = parseHelperMessage(data.data(), data.size());
    return true;
}

void PipeClient::startReadLoop(std::function<void(const HelperMessage&)> callback) {
    readThread_ = std::thread([this, callback]() {
        while (fd_ >= 0) {
            HelperMessage msg;
            if (!readMessage(msg)) break;
            callback(msg);
        }
    });
}

bool PipeClient::writeFrame(const std::vector<uint8_t>& data) {
    uint32_t len = uint32_t(data.size());
    uint8_t lenBuf[4];
    lenBuf[0] = uint8_t(len);
    lenBuf[1] = uint8_t(len >> 8);
    lenBuf[2] = uint8_t(len >> 16);
    lenBuf[3] = uint8_t(len >> 24);
    if (!writeAll(lenBuf, 4)) return false;
    if (!data.empty()) {
        if (!writeAll(data.data(), data.size())) return false;
    }
    return true;
}

bool PipeClient::readFrame(std::vector<uint8_t>& data) {
    uint8_t lenBuf[4];
    if (!readExact(lenBuf, 4)) return false;
    uint32_t len = uint32_t(lenBuf[0])
                 | (uint32_t(lenBuf[1]) << 8)
                 | (uint32_t(lenBuf[2]) << 16)
                 | (uint32_t(lenBuf[3]) << 24);
    if (len > 10 * 1024 * 1024) return false;
    data.resize(len);
    if (len > 0) {
        if (!readExact(data.data(), len)) return false;
    }
    return true;
}

bool PipeClient::writeAll(const uint8_t* data, size_t len) {
#ifdef _WIN32
    DWORD written;
    HANDLE h = (HANDLE)(intptr_t)fd_;
    while (len > 0) {
        if (!WriteFile(h, data, (DWORD)len, &written, nullptr)) return false;
        data += written;
        len -= written;
    }
    return true;
#else
    while (len > 0) {
        ssize_t n = write(fd_, data, len);
        if (n <= 0) return false;
        data += n;
        len -= size_t(n);
    }
    return true;
#endif
}

bool PipeClient::readExact(uint8_t* buf, size_t len) {
#ifdef _WIN32
    DWORD nread;
    HANDLE h = (HANDLE)(intptr_t)fd_;
    while (len > 0) {
        if (!ReadFile(h, buf, (DWORD)len, &nread, nullptr)) return false;
        if (nread == 0) return false;
        buf += nread;
        len -= nread;
    }
    return true;
#else
    while (len > 0) {
        ssize_t n = read(fd_, buf, len);
        if (n <= 0) return false;
        buf += size_t(n);
        len -= size_t(n);
    }
    return true;
#endif
}
