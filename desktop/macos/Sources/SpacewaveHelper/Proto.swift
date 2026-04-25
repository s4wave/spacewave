import Foundation

// Hand-written Swift types matching helper.proto wire format.
// These mirror the protobuf-go-lite generated types for IPC.
// Uses minimal protobuf wire format parsing, no SwiftProtobuf dependency.
//
// DRIFT WARNING: canonical source is the alpha repo at
//   core/launcher/helper/helper.proto
// (vendored here under vendor/github.com/s4wave/spacewave/
// core/launcher/helper/helper.proto). Any change to the proto
// message shapes, field numbers, or wire types MUST be mirrored by
// hand in the HelperMessage / HelperEvent structs and the
// parse*/serialize* functions below.

// HelperMessage is sent from Go to the native helper.
// Wire format: oneof body with field numbers 1-4.
struct HelperMessage {
    var progress: ProgressUpdate?
    var status: StatusUpdate?
    var dismiss: Bool
    var error: ErrorReport?

    init() {
        self.dismiss = false
    }
}

struct ProgressUpdate {
    var fraction: Float
    var text: String
}

struct StatusUpdate {
    var text: String
}

struct ErrorReport {
    var message: String
    var retryable: Bool
}

// HelperEvent is sent from the native helper back to Go.
struct HelperEvent {
    enum EventType {
        case retry
        case cancel
        case ready
    }
    var type_: EventType
}

// Minimal protobuf wire format parser for HelperMessage.
// Only needs to handle the specific message shapes we receive.
func parseHelperMessage(_ data: Data) -> HelperMessage {
    var msg = HelperMessage()
    var offset = 0

    while offset < data.count {
        let (fieldNumber, wireType, newOffset) = readTag(data, offset: offset)
        offset = newOffset

        switch (fieldNumber, wireType) {
        case (1, 2): // progress (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            msg.progress = parseProgressUpdate(subData)
        case (2, 2): // status (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            msg.status = parseStatusUpdate(subData)
        case (3, 2): // dismiss (length-delimited, empty message)
            let (_, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            msg.dismiss = true
        case (4, 2): // error (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            msg.error = parseErrorReport(subData)
        default:
            offset = skipField(data, offset: offset, wireType: wireType)
        }
    }
    return msg
}

func parseProgressUpdate(_ data: Data) -> ProgressUpdate {
    var fraction: Float = 0
    var text = ""
    var offset = 0

    while offset < data.count {
        let (fieldNumber, wireType, newOffset) = readTag(data, offset: offset)
        offset = newOffset

        switch (fieldNumber, wireType) {
        case (1, 5): // float (fixed32)
            fraction = readFloat(data, offset: offset)
            offset += 4
        case (2, 2): // string (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            text = String(data: subData, encoding: .utf8) ?? ""
        default:
            offset = skipField(data, offset: offset, wireType: wireType)
        }
    }
    return ProgressUpdate(fraction: fraction, text: text)
}

func parseStatusUpdate(_ data: Data) -> StatusUpdate {
    var text = ""
    var offset = 0

    while offset < data.count {
        let (fieldNumber, wireType, newOffset) = readTag(data, offset: offset)
        offset = newOffset

        switch (fieldNumber, wireType) {
        case (1, 2): // string (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            text = String(data: subData, encoding: .utf8) ?? ""
        default:
            offset = skipField(data, offset: offset, wireType: wireType)
        }
    }
    return StatusUpdate(text: text)
}

func parseErrorReport(_ data: Data) -> ErrorReport {
    var message = ""
    var retryable = false
    var offset = 0

    while offset < data.count {
        let (fieldNumber, wireType, newOffset) = readTag(data, offset: offset)
        offset = newOffset

        switch (fieldNumber, wireType) {
        case (1, 2): // string (length-delimited)
            let (subData, nextOffset) = readLengthDelimited(data, offset: offset)
            offset = nextOffset
            message = String(data: subData, encoding: .utf8) ?? ""
        case (2, 0): // bool (varint)
            let (value, nextOffset) = readVarint(data, offset: offset)
            offset = nextOffset
            retryable = value != 0
        default:
            offset = skipField(data, offset: offset, wireType: wireType)
        }
    }
    return ErrorReport(message: message, retryable: retryable)
}

// Serialize HelperEvent to protobuf wire format.
func serializeHelperEvent(_ event: HelperEvent) -> Data {
    var out = Data()
    switch event.type_ {
    case .retry:
        // field 1, wire type 2 (length-delimited), empty message
        out.append(contentsOf: [0x0a, 0x00])
    case .cancel:
        // field 2, wire type 2 (length-delimited), empty message
        out.append(contentsOf: [0x12, 0x00])
    case .ready:
        // field 3, wire type 2 (length-delimited), empty message
        out.append(contentsOf: [0x1a, 0x00])
    }
    return out
}

// Protobuf wire format helpers.

func readTag(_ data: Data, offset: Int) -> (fieldNumber: Int, wireType: Int, newOffset: Int) {
    let (value, newOffset) = readVarint(data, offset: offset)
    let fieldNumber = Int(value >> 3)
    let wireType = Int(value & 0x07)
    return (fieldNumber, wireType, newOffset)
}

func readVarint(_ data: Data, offset: Int) -> (UInt64, Int) {
    var result: UInt64 = 0
    var shift: UInt64 = 0
    var pos = offset
    while pos < data.count {
        let b = UInt64(data[pos])
        result |= (b & 0x7f) << shift
        pos += 1
        if b & 0x80 == 0 {
            break
        }
        shift += 7
    }
    return (result, pos)
}

func readLengthDelimited(_ data: Data, offset: Int) -> (Data, Int) {
    let (length, newOffset) = readVarint(data, offset: offset)
    let end = newOffset + Int(length)
    let subData = data[newOffset..<end]
    return (Data(subData), end)
}

func readFloat(_ data: Data, offset: Int) -> Float {
    let bytes = data[offset..<offset+4]
    return bytes.withUnsafeBytes { $0.load(as: Float.self) }
}

func skipField(_ data: Data, offset: Int, wireType: Int) -> Int {
    switch wireType {
    case 0: // varint
        let (_, newOffset) = readVarint(data, offset: offset)
        return newOffset
    case 1: // 64-bit
        return offset + 8
    case 2: // length-delimited
        let (_, newOffset) = readLengthDelimited(data, offset: offset)
        return newOffset
    case 5: // 32-bit
        return offset + 4
    default:
        return data.count // skip to end
    }
}
