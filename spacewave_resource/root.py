"""The Root Resource wrapper and its Session mount."""

from __future__ import annotations

from sdk.root import root_pb2
from sdk.root.root_srpc import RootResourceServiceClient

from .client import ResourceRef
from .errors import ResourceProtocolError
from .resource import Resource
from .session import Session


class Root(Resource):
    """Reach the Spacewave Root service over one held reference."""

    def __init__(self, resource_ref: ResourceRef) -> None:
        super().__init__(resource_ref)
        self._service = RootResourceServiceClient(resource_ref.client)

    async def mount_session_by_idx(self, session_idx: int) -> Session | None:
        """Mount the Session at session_idx, or return None when it is absent.

        NotFound is decisive. The Go Root reports it before it constructs a
        child, so a response that also carries a nonzero resource ID creates no
        child reference; TypeScript already reads the response this way.
        """
        response = await self._service.mount_session_by_idx(
            root_pb2.MountSessionByIdxRequest(session_idx=session_idx)
        )
        if response.not_found:
            return None
        if response.resource_id == 0:
            raise ResourceProtocolError("MountSessionByIdx returned an empty child ID")
        return Session(
            self._resource_ref.create_resource_reference(response.resource_id)
        )


__all__ = ["Root"]
