"""Typed Resource negotiation over StarPC's existing nested data transport."""

from __future__ import annotations

from collections.abc import (
    AsyncGenerator,
    AsyncIterable,
    AsyncIterator,
    Awaitable,
    Callable,
)

from rpcstream import rpcstream_pb2
from starpc.rpcstream import build_rpc_stream_open_stream
from starpc.stream import ByteStream

from bldr.resource import resource_pb2

from .errors import ResourceError, ResourceProtocolError


class ResourceFailureError(ResourceError):
    """A peer's typed lifecycle refusal, independent of its diagnostic wording."""

    def __init__(self, failure: resource_pb2.ResourceFailure) -> None:
        super().__init__(failure.message)
        self.code = failure.code


def build_resource_rpc_open_stream(
    resource_id: int,
    caller: Callable[
        [AsyncIterable[resource_pb2.ResourceRpcPacket]],
        AsyncIterator[resource_pb2.ResourceRpcPacket],
    ],
) -> Callable[[], Awaitable[ByteStream]]:
    """Open a typed Resource route, reusing StarPC framing and stream cleanup."""
    if not 0 < resource_id <= 0xFFFFFFFF:
        raise ValueError("invalid Resource ID")

    async def adapt(
        requests: AsyncIterable[rpcstream_pb2.RpcStreamPacket],
    ) -> AsyncIterator[rpcstream_pb2.RpcStreamPacket]:
        """Translate the local framing adapter; only Resource packets reach the wire."""

        async def outgoing() -> AsyncIterator[resource_pb2.ResourceRpcPacket]:
            async for packet in requests:
                if packet.WhichOneof("body") == "init":
                    yield resource_pb2.ResourceRpcPacket(
                        init=resource_pb2.ResourceRpcInit(resource_id=resource_id)
                    )
                else:
                    yield resource_pb2.ResourceRpcPacket(data=packet.data)

        responses = caller(outgoing())
        try:
            async for packet in responses:
                body = packet.WhichOneof("body")
                if body == "ack":
                    if packet.ack.HasField("failure"):
                        raise ResourceFailureError(packet.ack.failure)
                    yield rpcstream_pb2.RpcStreamPacket(ack=rpcstream_pb2.RpcAck())
                elif body == "data":
                    yield rpcstream_pb2.RpcStreamPacket(data=packet.data)
                else:
                    raise ResourceProtocolError("unexpected ResourceRpc packet")
        finally:
            if isinstance(responses, AsyncGenerator):
                await responses.aclose()

    return build_rpc_stream_open_stream(str(resource_id), adapt)
