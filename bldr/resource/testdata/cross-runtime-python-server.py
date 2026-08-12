"""Serve the Python Resource fixture over one StarPC RPC per TCP connection."""

from __future__ import annotations

import argparse
import asyncio
import contextlib
import struct
import sys
from collections.abc import AsyncGenerator, AsyncIterator, Awaitable, Callable

from rpcstream import rpcstream_pb2
from starpc.call import Call, CallError
from starpc.server import Server, ServiceRegistry
from starpc.stream import ByteStream

from bldr.resource import resource_pb2
from spacewave_resource import ResourceCall, ResourceFactory, ResourceServer

_ROOT_SERVICE = "test.Root"
_CHILD_SERVICE = "test.Child"


def report(marker: str) -> None:
    print(marker, flush=True)


class TcpByteStream(ByteStream):
    """One StarPC stream carried by one TCP connection."""

    def __init__(
        self,
        reader: asyncio.StreamReader,
        writer: asyncio.StreamWriter,
    ) -> None:
        self._reader = reader
        self._writer = writer
        self._closed = False

    async def read(self, max_bytes: int) -> bytes:
        return await self._reader.read(max_bytes)

    async def write(self, data: bytes) -> int:
        if self._closed:
            raise OSError("TCP stream is closed")
        self._writer.write(data)
        await self._writer.drain()
        return len(data)

    async def write_eof(self) -> None:
        if self._closed or self._writer.is_closing():
            return
        self._writer.write_eof()
        await self._writer.drain()

    async def aclose(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._writer.close()
        with contextlib.suppress(ConnectionError, OSError):
            await self._writer.wait_closed()


class CoreResourceServer(ResourceServer):
    """ResourceServer fixture with externally released lifecycle barriers."""

    def __init__(self, root_factory: ResourceFactory) -> None:
        super().__init__(root_factory)
        self.adopt_ack_gate = asyncio.Event()
        self.release_gate = asyncio.Event()
        self.adopt_ack_held = asyncio.Event()
        self.release_entered = asyncio.Event()
        self.generation_released = asyncio.Event()
        self.route_before_adopt_ack = False
        self._held_adopt = False

    async def _apply_control(
        self,
        generation: object,
        request: resource_pb2.ResourceClientRequest,
    ) -> None:
        if not self._held_adopt and request.WhichOneof("body") == "adopt":
            self._held_adopt = True
            self.adopt_ack_held.set()
            report("ADOPT_ACK_HELD")
            await self.adopt_ack_gate.wait()
        await super()._apply_control(generation, request)  # type: ignore[arg-type]

    async def resource_rpc(
        self,
        requests: AsyncIterator[rpcstream_pb2.RpcStreamPacket],
    ) -> AsyncGenerator[rpcstream_pb2.RpcStreamPacket, None]:
        if not self.adopt_ack_gate.is_set():
            self.route_before_adopt_ack = True
        async for response in super().resource_rpc(requests):
            yield response

    async def _release_generation(self, generation: object) -> None:
        await super()._release_generation(generation)  # type: ignore[arg-type]
        if not self._generations:
            self.generation_released.set()


class CoreDomain:
    """Minimal root and child Resource domain used by reciprocal fixtures."""

    def __init__(self, server: CoreResourceServer) -> None:
        self._server = server
        self.active_handlers = 0
        self.handlers_zero = asyncio.Event()
        self.handlers_zero.set()
        self.release_count = 0

    def root_factory(self, registry: ServiceRegistry, call: ResourceCall) -> None:
        registry.register(_ROOT_SERVICE, "Spawn", self._spawn(call))
        registry.register(_ROOT_SERVICE, "Echo", self._echo)

    def child_factory(self, registry: ServiceRegistry, call: ResourceCall) -> None:
        del call
        registry.register(_CHILD_SERVICE, "Echo", self._echo)
        registry.register(_CHILD_SERVICE, "Stream", self._stream)
        registry.register(_CHILD_SERVICE, "Block", self._block)

    def _spawn(self, resource_call: ResourceCall) -> Callable[[Call], Awaitable[None]]:
        async def handler(call: Call) -> None:
            self._require(await call.receive(), b"spawn")
            child = await resource_call.construct_child_resource(
                self.child_factory,
                self._release_child,
            )
            await call.send(struct.pack(">I", child.id))

        return handler

    async def _echo(self, call: Call) -> None:
        data = await call.receive()
        if data is None:
            raise AssertionError("Echo closed before request data")
        await call.send(data)

    async def _stream(self, call: Call) -> None:
        self._require(await call.receive(), b"stream")
        await call.send(b"later")

    async def _block(self, call: Call) -> None:
        self._require(await call.receive(), b"block")
        self.active_handlers += 1
        self.handlers_zero.clear()
        try:
            await call.send(b"active")
            await call.wait_aborted()
        except CallError:
            pass
        finally:
            self.active_handlers -= 1
            if self.active_handlers == 0:
                self.handlers_zero.set()
            report("HANDLER_FINALLY")

    async def _release_child(self) -> None:
        self._server.release_entered.set()
        report("RELEASE_ENTERED")
        await self._server.release_gate.wait()
        self.release_count += 1
        report("RELEASE_COMPLETE")

    @staticmethod
    def _require(value: bytes | None, want: bytes) -> None:
        if value != want:
            raise AssertionError(f"request = {value!r}, want {want!r}")


async def serve_connection(
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
    registry: ServiceRegistry,
) -> None:
    stream = TcpByteStream(reader, writer)
    try:
        await Server(registry).serve(stream)
    finally:
        await stream.aclose()


async def read_commands(server: CoreResourceServer) -> None:
    while True:
        command = await asyncio.to_thread(sys.stdin.readline)
        if not command:
            return
        match command.strip():
            case "ALLOW_ADOPT":
                server.adopt_ack_gate.set()
            case "ALLOW_RELEASE":
                server.release_gate.set()
            case "INVALIDATE":
                await server.aclose()
                return
            case other:
                raise ValueError(f"unknown fixture command: {other}")


async def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen", required=True)
    args = parser.parse_args()
    host, port_text = args.listen.rsplit(":", 1)

    placeholder: dict[str, CoreDomain] = {}
    server = CoreResourceServer(
        lambda registry, call: placeholder["domain"].root_factory(registry, call)
    )
    domain = CoreDomain(server)
    placeholder["domain"] = domain
    registry = ServiceRegistry()
    server.register(registry)
    listener = await asyncio.start_server(
        lambda reader, writer: serve_connection(reader, writer, registry),
        host,
        int(port_text),
    )
    address = listener.sockets[0].getsockname()
    report(f"READY {address[0]}:{address[1]}")

    try:
        await read_commands(server)
        await server.generation_released.wait()
        await domain.handlers_zero.wait()
        if server.route_before_adopt_ack:
            raise AssertionError(
                "ResourceRpc opened before delayed Adopt acknowledgement"
            )
        if server._generations:
            raise AssertionError("ResourceServer retained a generation")
        if domain.active_handlers != 0 or domain.release_count != 1:
            raise AssertionError(
                f"owner state = handlers:{domain.active_handlers} releases:{domain.release_count}"
            )
        report("PY_SERVER_OWNER_ZERO")
    finally:
        listener.close()
        await listener.wait_closed()
        await server.aclose()


if __name__ == "__main__":
    asyncio.run(main())
