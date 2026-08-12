"""The Session Resource wrapper and its Space list watch."""

from __future__ import annotations

from collections.abc import AsyncGenerator
from typing import cast

from sdk.session import session_pb2
from sdk.session.session_srpc import SessionResourceServiceClient

from .client import ResourceRef
from .resource import Resource


class Session(Resource):
    """Reach the Spacewave Session service over one held reference."""

    def __init__(self, resource_ref: ResourceRef) -> None:
        super().__init__(resource_ref)
        self._service = SessionResourceServiceClient(resource_ref.client)

    async def watch_resources_list(
        self,
    ) -> AsyncGenerator[session_pb2.WatchResourcesListResponse, None]:
        """Stream full Space list snapshots until the caller closes the stream.

        The Go Session owner derives its watch context from the stream and
        cancels that context only when the stream ends, so a caller awaits
        aclose() on this generator before it releases the Session.
        """
        # The generated server-streaming method is an async generator that
        # closes its StarPC call in a finally block, while its generated
        # annotation states only AsyncIterator.
        responses = cast(
            AsyncGenerator[session_pb2.WatchResourcesListResponse, None],
            self._service.watch_resources_list(session_pb2.WatchResourcesListRequest()),
        )
        try:
            async for response in responses:
                yield response
        finally:
            await responses.aclose()


__all__ = ["Session"]
