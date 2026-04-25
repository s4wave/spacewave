#pragma once
#include "proto.h"
#include <functional>
#include <string>
#include <thread>
#include <vector>

// PipeClient connects to the Go process via Unix socket or Windows named pipe.
// Uses 4-byte LE uint32 length-prefixed framing (framedstream protocol).
class PipeClient {
public:
    explicit PipeClient(const std::string& pipePath);
    ~PipeClient();

    bool connect();
    void close();

    bool sendEvent(const HelperEvent& event);
    bool readMessage(HelperMessage& msg);
    void startReadLoop(std::function<void(const HelperMessage&)> callback);

private:
    bool writeFrame(const std::vector<uint8_t>& data);
    bool readFrame(std::vector<uint8_t>& data);
    bool writeAll(const uint8_t* data, size_t len);
    bool readExact(uint8_t* buf, size_t len);

    std::string pipePath_;
    int fd_ = -1;
    std::thread readThread_;
};
