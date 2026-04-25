import Foundation

// PipeClient connects to the Go process via Unix domain socket
// using 4-byte LE uint32 length-prefixed framing (framedstream protocol).
class PipeClient {
    private var socket: Int32 = -1
    private let pipePath: String
    var onMessage: ((HelperMessage) -> Void)?

    init(pipePath: String) {
        self.pipePath = pipePath
    }

    func connect() throws {
        socket = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard socket >= 0 else {
            throw HelperError.socketCreate
        }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = pipePath.utf8CString
        guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
            throw HelperError.pathTooLong
        }
        withUnsafeMutablePointer(to: &addr.sun_path) { ptr in
            ptr.withMemoryRebound(to: CChar.self, capacity: pathBytes.count) { dest in
                for i in 0..<pathBytes.count {
                    dest[i] = pathBytes[i]
                }
            }
        }

        let addrLen = socklen_t(MemoryLayout<sockaddr_un>.size)
        let result = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockPtr in
                Darwin.connect(socket, sockPtr, addrLen)
            }
        }
        guard result == 0 else {
            throw HelperError.connectFailed(errno)
        }
    }

    func sendEvent(_ event: HelperEvent) throws {
        let data = serializeHelperEvent(event)
        try writeFrame(data)
    }

    func readMessage() throws -> HelperMessage {
        let data = try readFrame()
        return parseHelperMessage(data)
    }

    func startReadLoop() {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            guard let self = self else { return }
            while self.socket >= 0 {
                do {
                    let msg = try self.readMessage()
                    DispatchQueue.main.async {
                        self.onMessage?(msg)
                    }
                } catch {
                    // Connection closed or error, exit read loop.
                    break
                }
            }
        }
    }

    func close() {
        if socket >= 0 {
            Darwin.close(socket)
            socket = -1
        }
    }

    // Write a frame: 4-byte LE uint32 length prefix + data.
    private func writeFrame(_ data: Data) throws {
        var length = UInt32(data.count).littleEndian
        let lenData = Data(bytes: &length, count: 4)
        try writeAll(lenData)
        try writeAll(data)
    }

    // Read a frame: 4-byte LE uint32 length prefix + data.
    private func readFrame() throws -> Data {
        let lenData = try readExact(4)
        let length = lenData.withUnsafeBytes { $0.load(as: UInt32.self) }
        let msgLen = UInt32(littleEndian: length)
        guard msgLen <= 10 * 1024 * 1024 else {
            throw HelperError.messageTooLarge
        }
        return try readExact(Int(msgLen))
    }

    private func writeAll(_ data: Data) throws {
        try data.withUnsafeBytes { ptr in
            var remaining = data.count
            var offset = 0
            while remaining > 0 {
                let written = Darwin.write(socket, ptr.baseAddress!.advanced(by: offset), remaining)
                guard written > 0 else {
                    throw HelperError.writeFailed
                }
                offset += written
                remaining -= written
            }
        }
    }

    private func readExact(_ count: Int) throws -> Data {
        var buffer = Data(count: count)
        var remaining = count
        var offset = 0
        while remaining > 0 {
            let n = buffer.withUnsafeMutableBytes { ptr in
                Darwin.read(socket, ptr.baseAddress!.advanced(by: offset), remaining)
            }
            guard n > 0 else {
                throw HelperError.readFailed
            }
            offset += n
            remaining -= n
        }
        return buffer
    }
}

enum HelperError: Error {
    case socketCreate
    case pathTooLong
    case connectFailed(Int32)
    case messageTooLarge
    case writeFailed
    case readFailed
}
