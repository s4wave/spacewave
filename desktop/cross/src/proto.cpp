#include "proto.h"
#include <cstring>

// Protobuf wire format helpers.

static size_t readVarint(const uint8_t* data, size_t offset, size_t len, uint64_t& out) {
    out = 0;
    uint64_t shift = 0;
    size_t pos = offset;
    while (pos < len) {
        uint8_t b = data[pos];
        out |= (uint64_t(b & 0x7f)) << shift;
        pos++;
        if ((b & 0x80) == 0) break;
        shift += 7;
    }
    return pos;
}

static size_t readTag(const uint8_t* data, size_t offset, size_t len,
                      int& fieldNumber, int& wireType) {
    uint64_t val;
    size_t pos = readVarint(data, offset, len, val);
    fieldNumber = int(val >> 3);
    wireType = int(val & 0x07);
    return pos;
}

static size_t readLengthDelimited(const uint8_t* data, size_t offset, size_t len,
                                  const uint8_t*& out, size_t& outLen) {
    uint64_t length;
    size_t pos = readVarint(data, offset, len, length);
    out = data + pos;
    outLen = size_t(length);
    return pos + outLen;
}

static size_t skipField(const uint8_t* data, size_t offset, size_t len, int wireType) {
    switch (wireType) {
    case 0: { // varint
        uint64_t dummy;
        return readVarint(data, offset, len, dummy);
    }
    case 1: return offset + 8;
    case 2: {
        const uint8_t* dummy;
        size_t dummyLen;
        return readLengthDelimited(data, offset, len, dummy, dummyLen);
    }
    case 5: return offset + 4;
    default: return len;
    }
}

static ProgressUpdate parseProgressUpdate(const uint8_t* data, size_t len) {
    ProgressUpdate p;
    size_t offset = 0;
    while (offset < len) {
        int fn, wt;
        offset = readTag(data, offset, len, fn, wt);
        if (fn == 1 && wt == 5) {
            // float (fixed32)
            float val;
            memcpy(&val, data + offset, 4);
            p.fraction = val;
            offset += 4;
        } else if (fn == 2 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            p.text.assign(reinterpret_cast<const char*>(sub), subLen);
        } else {
            offset = skipField(data, offset, len, wt);
        }
    }
    return p;
}

static StatusUpdate parseStatusUpdate(const uint8_t* data, size_t len) {
    StatusUpdate s;
    size_t offset = 0;
    while (offset < len) {
        int fn, wt;
        offset = readTag(data, offset, len, fn, wt);
        if (fn == 1 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            s.text.assign(reinterpret_cast<const char*>(sub), subLen);
        } else {
            offset = skipField(data, offset, len, wt);
        }
    }
    return s;
}

static ErrorReport parseErrorReport(const uint8_t* data, size_t len) {
    ErrorReport e;
    size_t offset = 0;
    while (offset < len) {
        int fn, wt;
        offset = readTag(data, offset, len, fn, wt);
        if (fn == 1 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            e.message.assign(reinterpret_cast<const char*>(sub), subLen);
        } else if (fn == 2 && wt == 0) {
            uint64_t val;
            offset = readVarint(data, offset, len, val);
            e.retryable = val != 0;
        } else {
            offset = skipField(data, offset, len, wt);
        }
    }
    return e;
}

HelperMessage parseHelperMessage(const uint8_t* data, size_t len) {
    HelperMessage msg;
    size_t offset = 0;
    while (offset < len) {
        int fn, wt;
        offset = readTag(data, offset, len, fn, wt);
        if (fn == 1 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            msg.type = MessageType::Progress;
            msg.progress = parseProgressUpdate(sub, subLen);
        } else if (fn == 2 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            msg.type = MessageType::Status;
            msg.status = parseStatusUpdate(sub, subLen);
        } else if (fn == 3 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            msg.type = MessageType::Dismiss;
        } else if (fn == 4 && wt == 2) {
            const uint8_t* sub;
            size_t subLen;
            offset = readLengthDelimited(data, offset, len, sub, subLen);
            msg.type = MessageType::Error;
            msg.error = parseErrorReport(sub, subLen);
        } else {
            offset = skipField(data, offset, len, wt);
        }
    }
    return msg;
}

std::vector<uint8_t> serializeHelperEvent(const HelperEvent& event) {
    std::vector<uint8_t> out;
    switch (event.type) {
    case EventType::Retry:
        // field 1, wire type 2, length 0
        out.push_back(0x0a);
        out.push_back(0x00);
        break;
    case EventType::Cancel:
        // field 2, wire type 2, length 0
        out.push_back(0x12);
        out.push_back(0x00);
        break;
    case EventType::Ready:
        // field 3, wire type 2, length 0
        out.push_back(0x1a);
        out.push_back(0x00);
        break;
    }
    return out;
}
