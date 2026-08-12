from __future__ import annotations

import asyncio
import contextlib
import unittest
from collections.abc import AsyncIterator
from typing import Final, cast

from _resource_fixture import ResourceServerHarness
from starpc.server import ServiceRegistry

from core.provider import provider_pb2
from core.sobject import sobject_pb2
from core.space import space_pb2
from sdk.root import root_pb2
from sdk.root.root_srpc import RootResourceServiceServer, register_root_resource_service
from sdk.session import session_pb2
from sdk.session.session_srpc import (
    SessionResourceServiceServer,
    register_session_resource_service,
)
from spacewave_resource import (
    ResourceCall,
    ResourceClient,
    ResourceFactory,
    ResourceProtocolError,
    Root,
    Session,
)

MOUNTED_SESSION_IDX: Final = 4
MISSING_SESSION_IDX: Final = 7
EMPTY_CHILD_SESSION_IDX: Final = 9
# The Go Root reports NotFound before it constructs a child, so a response
# carrying both facts is contradictory. TypeScript already treats NotFound as
# decisive, and this fixture holds Python to the same precedence.
CONTRADICTORY_RESOURCE_ID: Final = 4242
SPACE_NAME: Final = "Terminal"
SPACE_RESOURCE_ID: Final = "terminal-id"


class SessionOwnerFixture:
    """One Session watch that ends only when its client closes the stream."""

    def __init__(self) -> None:
        self.watch_canceled = asyncio.Event()
        self.watch_exited = asyncio.Event()
        self._stream_open = asyncio.Event()

    async def watch_resources_list(
        self, request: session_pb2.WatchResourcesListRequest
    ) -> AsyncIterator[session_pb2.WatchResourcesListResponse]:
        del request
        try:
            yield session_pb2.WatchResourcesListResponse(
                spaces_list=[
                    space_pb2.SpaceSoListEntry(
                        entry=sobject_pb2.SharedObjectListEntry(
                            ref=sobject_pb2.SharedObjectRef(
                                provider_resource_ref=provider_pb2.ProviderResourceRef(
                                    id=SPACE_RESOURCE_ID
                                )
                            )
                        ),
                        space_meta=space_pb2.SpaceSoMeta(name=SPACE_NAME),
                    )
                ]
            )
            # The Go Session owner derives its watch context from the stream, so
            # the handler stays inside user code until the client cancels.
            await self._stream_open.wait()
        except asyncio.CancelledError:
            self.watch_canceled.set()
            raise
        finally:
            self.watch_exited.set()


class RootFixture:
    """One Root Resource that mounts the fixture Session by index."""

    def __init__(
        self, resource_call: ResourceCall, session: SessionOwnerFixture
    ) -> None:
        self._resource_call = resource_call
        self._session = session

    async def mount_session_by_idx(
        self, request: root_pb2.MountSessionByIdxRequest
    ) -> root_pb2.MountSessionByIdxResponse:
        if request.session_idx == MISSING_SESSION_IDX:
            return root_pb2.MountSessionByIdxResponse(
                not_found=True, resource_id=CONTRADICTORY_RESOURCE_ID
            )
        if request.session_idx == EMPTY_CHILD_SESSION_IDX:
            return root_pb2.MountSessionByIdxResponse()
        child = await self._resource_call.construct_child_resource(
            self._register_session
        )
        return root_pb2.MountSessionByIdxResponse(resource_id=child.id)

    def _register_session(
        self, registry: ServiceRegistry, resource_call: ResourceCall
    ) -> None:
        del resource_call
        register_session_resource_service(
            registry, cast(SessionResourceServiceServer, self._session)
        )


def build_root_factory(session: SessionOwnerFixture) -> ResourceFactory:
    """Build the harness root factory that serves the generated Root service."""

    def register(registry: ServiceRegistry, resource_call: ResourceCall) -> None:
        # The generated registrar resolves each implementation method only when
        # that method is invoked, so this journey fixture serves MountSessionByIdx
        # alone.
        register_root_resource_service(
            registry,
            cast(RootResourceServiceServer, RootFixture(resource_call, session)),
        )

    return register


class SessionJourneyTest(unittest.IsolatedAsyncioTestCase):
    async def test_root_mounts_session_and_streams_one_space_snapshot(self) -> None:
        session_owner = SessionOwnerFixture()
        harness = ResourceServerHarness(build_root_factory(session_owner))
        client = await ResourceClient.open(harness.service)
        try:
            root = Root(client.access_root_resource())

            self.assertIsNone(await root.mount_session_by_idx(MISSING_SESSION_IDX))
            self.assertNotIn(CONTRADICTORY_RESOURCE_ID, client._resources)
            generation = next(iter(harness.server._generations.values()))
            self.assertEqual(
                [
                    resource.id
                    for resource in generation.resources.values()
                    if resource.pending
                ],
                [],
            )

            with self.assertRaises(ResourceProtocolError):
                await root.mount_session_by_idx(EMPTY_CHILD_SESSION_IDX)

            session = await root.mount_session_by_idx(MOUNTED_SESSION_IDX)
            self.assertIsInstance(session, Session)
            assert session is not None

            stream = session.watch_resources_list()
            snapshot = await anext(stream)
            entry = snapshot.spaces_list[0]
            self.assertEqual(entry.space_meta.name, SPACE_NAME)
            self.assertEqual(
                entry.entry.ref.provider_resource_ref.id, SPACE_RESOURCE_ID
            )

            await stream.aclose()
            await session_owner.watch_exited.wait()
            self.assertTrue(session_owner.watch_canceled.is_set())

            await session.release()
            await root.release()
            await client.aclose()

            self.assertFalse(client._resources)
            self.assertFalse(client._active_routes)
            self.assertFalse(client._opening_routes)
            self.assertFalse(client._control_waiters)
            self.assertFalse(client._release_tasks)
            self.assertFalse(harness.server._generations)
            self.assertFalse(harness.server._handler_routes)
            self.assertEqual([task for task in harness.tasks if not task.done()], [])
        finally:
            with contextlib.suppress(Exception):
                await client.aclose()
            await harness.aclose()


if __name__ == "__main__":
    unittest.main()
