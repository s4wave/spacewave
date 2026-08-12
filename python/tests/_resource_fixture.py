from __future__ import annotations

import asyncio
import contextlib
from collections.abc import AsyncGenerator, AsyncIterator
from typing import Final, cast

from rpcstream import rpcstream_pb2
from starpc.client import Client
from starpc.rpcstream import build_rpc_stream_open_stream
from starpc.server import Server, ServiceRegistry
from starpc.stream import ByteStream, StreamClosedError, memory_stream_pair

from bldr.resource import resource_pb2
from bldr.resource.resource_srpc import (
    ResourceServiceClient,
    ResourceServiceServer,
    register_resource_service,
)
from spacewave_resource import ResourceFactory, ResourceServer

_END: Final = object()

ALPHA_SERVICE: Final = "test.Alpha"
BETA_SERVICE: Final = "test.Beta"


class ResourceFixture(ResourceServiceServer):
    """In-process ResourceService with event-controlled lifecycle replies."""

    def __init__(self) -> None:
        self.controls: list[resource_pb2.ResourceClientRequest] = []
        self.routes: list[rpcstream_pb2.RpcStreamPacket] = []
        self._control_seen: asyncio.Queue[resource_pb2.ResourceClientRequest] = (
            asyncio.Queue()
        )
        self._responses: asyncio.Queue[resource_pb2.ResourceClientResponse | object] = (
            asyncio.Queue()
        )
        self._ack_gates: dict[int, asyncio.Event] = {}
        self.route_started = asyncio.Event()
        self.route_data = asyncio.Event()
        self.route_closed = asyncio.Event()
        self._route_ack = asyncio.Event()
        self._route_ack.set()
        self._streams: list[ByteStream] = []
        self._tasks: list[asyncio.Task[None]] = []
        registry = ServiceRegistry()
        register_resource_service(registry, self)

        async def opener() -> ByteStream:
            client_stream, server_stream = memory_stream_pair(4096)
            self._streams.extend((client_stream, server_stream))
            self._tasks.append(
                asyncio.create_task(Server(registry).serve(server_stream))
            )
            return client_stream

        self.service = ResourceServiceClient(Client(opener))

    def delay_ack(self, control_id: int) -> asyncio.Event:
        gate = asyncio.Event()
        self._ack_gates[control_id] = gate
        return gate

    def delay_route_ack(self) -> asyncio.Event:
        self._route_ack.clear()
        return self._route_ack

    async def next_control(self) -> resource_pb2.ResourceClientRequest:
        return await self._control_seen.get()

    async def send_release(self, resource_id: int) -> None:
        await self._responses.put(
            resource_pb2.ResourceClientResponse(
                resource_released=resource_pb2.ResourceReleasedResponse(
                    resource_id=resource_id
                )
            )
        )

    async def send_ack(self, control_id: int) -> None:
        await self._responses.put(
            resource_pb2.ResourceClientResponse(
                control_ack=resource_pb2.ResourceClientControlAck(control_id=control_id)
            )
        )

    async def disconnect(self) -> None:
        await self._responses.put(_END)

    async def resource_client(
        self,
        requests: AsyncIterator[resource_pb2.ResourceClientRequest],
    ) -> AsyncIterator[resource_pb2.ResourceClientResponse]:
        producer = asyncio.create_task(self._produce_controls(requests))
        try:
            while True:
                response = await self._responses.get()
                if response is _END:
                    return
                assert isinstance(response, resource_pb2.ResourceClientResponse)
                yield response
        finally:
            if not producer.done():
                producer.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await producer

    async def _produce_controls(
        self,
        requests: AsyncIterator[resource_pb2.ResourceClientRequest],
    ) -> None:
        try:
            async for request in requests:
                self.controls.append(request)
                await self._control_seen.put(request)
                body = request.WhichOneof("body")
                if body == "init":
                    await self._responses.put(
                        resource_pb2.ResourceClientResponse(
                            init=resource_pb2.ResourceClientInit(
                                client_handle_id=7,
                                root_resource_id=11,
                            )
                        )
                    )
                    continue
                gate = self._ack_gates.get(request.control_id)
                if gate is not None:
                    await gate.wait()
                await self.send_ack(request.control_id)
        finally:
            await self._responses.put(_END)

    async def resource_rpc(
        self,
        requests: AsyncIterator[rpcstream_pb2.RpcStreamPacket],
    ) -> AsyncIterator[rpcstream_pb2.RpcStreamPacket]:
        try:
            first = await anext(requests)
            self.routes.append(first)
            self.route_started.set()
            await self._route_ack.wait()
            yield rpcstream_pb2.RpcStreamPacket(ack=rpcstream_pb2.RpcAck())
            async for request in requests:
                self.routes.append(request)
                self.route_data.set()
        finally:
            self.route_closed.set()

    async def resource_attach(
        self,
        requests: AsyncIterator[resource_pb2.ResourceAttachRequest],
    ) -> AsyncIterator[resource_pb2.ResourceAttachResponse]:
        async for _request in requests:
            return
        if False:
            yield resource_pb2.ResourceAttachResponse()

    async def aclose(self) -> None:
        await self.disconnect()
        for stream in self._streams:
            await stream.aclose()
        for task in self._tasks:
            if not task.done():
                task.cancel()
        for task in self._tasks:
            with contextlib.suppress(asyncio.CancelledError, StreamClosedError):
                await task


class ResourceServerHarness:
    """Serve one ResourceServer to in-process generated Resource clients."""

    def __init__(self, root_factory: ResourceFactory) -> None:
        self.server = ResourceServer(root_factory)
        registry = ServiceRegistry()
        self.server.register(registry)
        # Each opened stream serves exactly one outer RPC, so the task recorded
        # here settles only after that RPC's handler has fully returned.
        self.tasks: list[asyncio.Task[None]] = []
        self._streams: list[ByteStream] = []

        async def opener() -> ByteStream:
            client_stream, server_stream = memory_stream_pair(65536)
            self._streams.extend((client_stream, server_stream))

            async def serve_one() -> None:
                with contextlib.suppress(StreamClosedError):
                    await Server(registry).serve(server_stream)

            self.tasks.append(asyncio.create_task(serve_one()))
            return client_stream

        self.service = ResourceServiceClient(Client(opener))

    def open_route_client(self, resource_id: int) -> Client:
        """Create a StarPC client whose calls route to one resource ID."""
        return Client(
            build_rpc_stream_open_stream(str(resource_id), self.service.resource_rpc)
        )

    async def aclose(self) -> None:
        await self.server.aclose()
        for stream in self._streams:
            await stream.aclose()
        for task in self.tasks:
            if not task.done():
                task.cancel()
        for task in self.tasks:
            with contextlib.suppress(asyncio.CancelledError, StreamClosedError):
                await task


class ResourceControlStream:
    """Drive one raw ResourceClient generation for server protocol tests."""

    def __init__(self, service: ResourceServiceClient) -> None:
        self._requests: asyncio.Queue[resource_pb2.ResourceClientRequest | object] = (
            asyncio.Queue()
        )
        self._responses = cast(
            AsyncGenerator[resource_pb2.ResourceClientResponse, None],
            service.resource_client(self._iter_requests()),
        )
        self.client_handle_id = 0
        self.root_resource_id = 0

    async def open(self) -> resource_pb2.ResourceClientInit:
        """Send Init with control ID zero and record the generation identity."""
        self.send(
            resource_pb2.ResourceClientRequest(
                control_id=0, init=resource_pb2.ResourceClientInitRequest()
            )
        )
        response = await self.receive()
        self.client_handle_id = response.init.client_handle_id
        self.root_resource_id = response.init.root_resource_id
        return response.init

    def send(self, request: resource_pb2.ResourceClientRequest) -> None:
        self._requests.put_nowait(request)

    def adopt(self, control_id: int, resource_id: int) -> None:
        self.send(
            resource_pb2.ResourceClientRequest(
                control_id=control_id,
                adopt=resource_pb2.ResourceClientAdopt(resource_id=resource_id),
            )
        )

    def release(self, control_id: int, resource_id: int) -> None:
        self.send(
            resource_pb2.ResourceClientRequest(
                control_id=control_id,
                release=resource_pb2.ResourceClientRelease(resource_id=resource_id),
            )
        )

    async def receive(self) -> resource_pb2.ResourceClientResponse:
        return await anext(self._responses)

    async def aclose(self) -> None:
        """End this generation and drain every remaining server response."""
        self._requests.put_nowait(_END)
        async for _response in self._responses:
            pass

    async def _iter_requests(self) -> AsyncIterator[resource_pb2.ResourceClientRequest]:
        while True:
            item = await self._requests.get()
            if item is _END:
                return
            assert isinstance(item, resource_pb2.ResourceClientRequest)
            yield item
