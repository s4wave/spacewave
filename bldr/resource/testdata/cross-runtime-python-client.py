"""Drive the Python Resource client against a reciprocal TCP fixture."""

from __future__ import annotations

import argparse
import asyncio
import contextlib
import struct

from starpc.call import Call, CallCancelledError, ClosedBeforeCompletionError
from starpc.client import Client
from starpc.stream import ByteStream

from bldr.resource.resource_srpc import ResourceServiceClient
from spacewave_resource import ResourceClient, ResourceRef, ResourceTerminalError

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


async def next_turn() -> None:
    """Yield one event-loop turn without a timer or a poll."""
    done = asyncio.get_running_loop().create_future()
    asyncio.get_running_loop().call_soon(done.set_result, None)
    await done


async def round_trip(ref: ResourceRef, service: str, method: str, data: bytes) -> bytes:
    client = ref.client
    call = await client.open_call(service, method)
    try:
        await call.send(data)
        await call.finish()
        response = await call.receive()
        if response is None:
            raise AssertionError(f"{method} completed without response data")
        if await call.receive() is not None:
            raise AssertionError(f"{method} streamed an unexpected second response")
        return response
    finally:
        await call.aclose()


async def open_active_block(ref: ResourceRef) -> Call:
    client = ref.client
    call = await client.open_call(_CHILD_SERVICE, "Block")
    await call.send(b"block")
    if await call.receive() != b"active":
        raise AssertionError("Block did not publish active data")
    return call


async def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("address")
    parser.add_argument(
        "--finish", choices=("close", "invalidate"), default="invalidate"
    )
    args = parser.parse_args()
    host, port_text = args.address.rsplit(":", 1)

    async def opener() -> ByteStream:
        reader, writer = await asyncio.open_connection(host, int(port_text))
        return TcpByteStream(reader, writer)

    client = await ResourceClient.open(ResourceServiceClient(Client(opener)))
    try:
        root = client.access_root_resource()
        child_data = await round_trip(root, _ROOT_SERVICE, "Spawn", b"spawn")
        child_id = struct.unpack(">I", child_data)[0]
        if child_id == 0:
            raise AssertionError("Spawn returned an empty child ID")
        child = client.create_resource_reference(child_id)

        if await round_trip(child, _CHILD_SERVICE, "Stream", b"stream") != b"later":
            raise AssertionError("Stream did not deliver later data")

        canceled = await open_active_block(child)
        await canceled.cancel()
        with contextlib.suppress(CallCancelledError):
            await canceled.receive()
        await canceled.aclose()

        active = await open_active_block(child)
        releasing = asyncio.create_task(child.release())
        with contextlib.suppress(ClosedBeforeCompletionError):
            await active.receive()
        await active.aclose()
        await next_turn()
        after_release = asyncio.create_task(
            round_trip(root, _ROOT_SERVICE, "Echo", b"after-release")
        )
        if await after_release != b"after-release":
            raise AssertionError(
                "Root route did not wait for the release acknowledgement"
            )
        await releasing

        await root.release()
        reused = client.access_root_resource()
        if await round_trip(reused, _ROOT_SERVICE, "Echo", b"reused") != b"reused":
            raise AssertionError("retained root did not route after release")
        await reused.release()

        if args.finish == "close":
            await client.aclose()
        else:
            report("PY_CLIENT_READY_TO_INVALIDATE")
            response_task = client._response_task
            if response_task is None:
                raise AssertionError("ResourceClient response task is absent")
            await response_task
            if not isinstance(client._terminal_error, ResourceTerminalError):
                raise AssertionError(
                    "server invalidation did not retire the generation"
                )
            try:
                client.access_root_resource()
            except ResourceTerminalError:
                pass
            else:
                raise AssertionError("invalidated root remained usable")
            await client.aclose()
        if (
            client._resources
            or client._active_routes
            or client._opening_routes
            or client._control_waiters
            or client._release_tasks
        ):
            raise AssertionError("Python client retained lifecycle state")
        report("PY_CLIENT_OWNER_ZERO")
    finally:
        with contextlib.suppress(Exception):
            await client.aclose()


if __name__ == "__main__":
    asyncio.run(main())
