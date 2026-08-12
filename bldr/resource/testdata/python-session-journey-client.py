"""Drive the Python Session journey against a real Go Session owner."""

from __future__ import annotations

import argparse
import asyncio
import contextlib

from starpc.client import Client
from starpc.stream import ByteStream

from bldr.resource.resource_srpc import ResourceServiceClient
from spacewave_resource import ResourceClient, Root

_SESSION_IDX = 4


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


async def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("address")
    args = parser.parse_args()
    host, port_text = args.address.rsplit(":", 1)

    async def opener() -> ByteStream:
        reader, writer = await asyncio.open_connection(host, int(port_text))
        return TcpByteStream(reader, writer)

    client = await ResourceClient.open(ResourceServiceClient(Client(opener)))
    try:
        root = Root(client.access_root_resource())
        session = await root.mount_session_by_idx(_SESSION_IDX)
        if session is None:
            raise AssertionError(f"session {_SESSION_IDX} was not found")

        # The Go Session owner publishes an empty snapshot before its shared
        # object list loads, so the journey reads until the first Space appears.
        stream = session.watch_resources_list()
        entry = None
        async for snapshot in stream:
            if snapshot.spaces_list:
                entry = snapshot.spaces_list[0]
                break
        if entry is None:
            raise AssertionError("Session watch closed before listing a Space")
        report(f"PY_SPACE_NAME {entry.space_meta.name}")
        report(f"PY_SPACE_ID {entry.entry.ref.provider_resource_ref.id}")
        await stream.aclose()

        await session.release()
        await root.release()
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
