#pragma once
#include <cstdint>
#include <string>
#include <vector>

// Minimal protobuf wire format types matching helper.proto.
// Hand-rolled to avoid depending on the full protobuf C++ runtime.

struct ProgressUpdate {
    float fraction = 0;
    std::string text;
};

struct StatusUpdate {
    std::string text;
};

struct ErrorReport {
    std::string message;
    bool retryable = false;
};

enum class MessageType {
    None,
    Progress,
    Status,
    Dismiss,
    Error
};

struct HelperMessage {
    MessageType type = MessageType::None;
    ProgressUpdate progress;
    StatusUpdate status;
    ErrorReport error;
};

enum class EventType {
    Retry,
    Cancel,
    Ready
};

struct HelperEvent {
    EventType type;
};

// Parse a HelperMessage from raw protobuf bytes.
HelperMessage parseHelperMessage(const uint8_t* data, size_t len);

// Serialize a HelperEvent to protobuf bytes.
std::vector<uint8_t> serializeHelperEvent(const HelperEvent& event);
