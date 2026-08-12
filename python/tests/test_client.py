from __future__ import annotations

import asyncio
import unittest

from _resource_fixture import ResourceFixture
from starpc.call import CallError

from bldr.resource.resource_srpc import ResourceServiceClient
from spacewave_resource import (
    ResourceClient,
    ResourceProtocolError,
    ResourceReleasedError,
    ResourceTerminalError,
)


async def yield_turn() -> None:
    """Yield exactly one event-loop turn without polling or a timer."""
    done = asyncio.get_running_loop().create_future()
    asyncio.get_running_loop().call_soon(done.set_result, None)
    await done


class _JoinGatedResourceClient(ResourceClient):
    def __init__(self, service: ResourceServiceClient) -> None:
        super().__init__(service)
        self.join_started = asyncio.Event()
        self.join_release = asyncio.Event()

    async def _join_release_tasks(self) -> None:
        self.join_started.set()
        await self.join_release.wait()
        await super()._join_release_tasks()


class ResourceClientTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.fixture = ResourceFixture()
        self.client: ResourceClient | None = None

    async def asyncTearDown(self) -> None:
        if self.client is not None:
            await self.client.aclose()
        await self.fixture.aclose()

    async def open_client(self) -> ResourceClient:
        self.client = await ResourceClient.open(self.fixture.service)
        return self.client

    async def test_init_is_first_and_controls_are_strictly_ordered(self) -> None:
        client = await self.open_client()
        init = await self.fixture.next_control()
        self.assertEqual(init.WhichOneof("body"), "init")
        self.assertEqual(init.control_id, 0)
        self.assertEqual(client.client_handle_id, 7)
        self.assertEqual(client.root_resource_id, 11)

        root = client.access_root_resource()
        adopt = await self.fixture.next_control()
        self.assertEqual((adopt.WhichOneof("body"), adopt.control_id), ("adopt", 1))
        self.assertEqual(adopt.adopt.resource_id, 11)
        await root.release()
        release = await self.fixture.next_control()
        self.assertEqual(
            (release.WhichOneof("body"), release.control_id), ("release", 2)
        )
        self.assertEqual(release.release.resource_id, 11)

    async def test_concurrent_clones_share_one_adopt_and_one_final_release(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        first = client.create_resource_reference(23)
        second = first.clone()
        adopt = await self.fixture.next_control()
        self.assertEqual((adopt.WhichOneof("body"), adopt.control_id), ("adopt", 1))
        self.assertEqual(adopt.adopt.resource_id, 23)

        await asyncio.gather(first.release(), second.release())
        release = await self.fixture.next_control()
        self.assertEqual(
            (release.WhichOneof("body"), release.control_id), ("release", 2)
        )
        self.assertEqual(release.release.resource_id, 23)
        self.assertFalse(client._resources)

    async def test_routed_client_waits_for_adopt_ack_and_release_aborts_open(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        adopt_ack = self.fixture.delay_ack(1)
        ref = client.create_resource_reference(31)
        await self.fixture.next_control()
        opening = asyncio.create_task(ref.client.open_call("test.Service", "Wait"))
        await yield_turn()
        self.assertFalse(self.fixture.route_started.is_set())

        route_ack = self.fixture.delay_route_ack()
        adopt_ack.set()
        await self.fixture.route_started.wait()
        releasing = asyncio.create_task(ref.release())
        await yield_turn()
        self.assertFalse(releasing.done())
        route_ack.set()
        with self.assertRaises(ResourceReleasedError):
            await opening
        await releasing
        self.assertFalse(client._active_routes)
        self.assertFalse(client._opening_routes)

    async def test_dropped_unacknowledged_route_does_not_block_release_or_close(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        ref = client.create_resource_reference(35)
        await self.fixture.next_control()
        self.fixture.delay_route_ack()
        opening = asyncio.create_task(ref.client.open_call("test.Service", "Dropped"))
        await self.fixture.route_started.wait()

        releasing = asyncio.create_task(ref.release())
        await yield_turn()
        closing = asyncio.create_task(client.aclose())
        await asyncio.gather(releasing, closing)
        with self.assertRaises(ResourceTerminalError):
            await opening

        self.client = None
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._pending_control_ids)
        self.assertFalse(client._opening_routes)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._release_tasks)
        self.assertIsNotNone(client._close_task)
        assert client._close_task is not None
        self.assertTrue(client._close_task.done())
        self.assertIsNotNone(client._response_task)
        assert client._response_task is not None
        self.assertTrue(client._response_task.done())

    async def test_cached_routed_client_cannot_open_after_final_release(self) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        ref = client.create_resource_reference(37)
        await self.fixture.next_control()
        routed = ref.client
        await ref.release()
        with self.assertRaises(ResourceReleasedError):
            await routed.open_call("test.Service", "Never")
        self.assertFalse(client._resources)

    async def test_bad_ack_terminally_retires_generation(self) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        self.fixture.delay_ack(1)
        ref = client.create_resource_reference(41)
        await self.fixture.next_control()
        opening = asyncio.create_task(ref.client.open_call("test.Service", "Wait"))
        await yield_turn()
        await self.fixture.send_ack(0)
        with self.assertRaises(ResourceProtocolError):
            await opening
        self.assertIsInstance(client._terminal_error, ResourceProtocolError)
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._active_routes)

    async def test_server_release_retires_refs_and_routes_without_control_replay(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        adopt_ack = self.fixture.delay_ack(1)
        ref = client.create_resource_reference(43)
        await self.fixture.next_control()
        opening = asyncio.create_task(ref.client.open_call("test.Service", "Wait"))
        await yield_turn()
        await self.fixture.send_release(43)
        with self.assertRaises(ResourceReleasedError):
            await opening
        await ref.release()
        self.assertFalse(client._control_waiters)
        adopt_ack.set()

        next_ref = client.create_resource_reference(59)
        next_adopt = await self.fixture.next_control()
        self.assertEqual(
            (next_adopt.WhichOneof("body"), next_adopt.control_id), ("adopt", 2)
        )
        self.assertEqual(next_adopt.adopt.resource_id, 59)
        next_call = await next_ref.client.open_call("test.Service", "Next")
        await next_call.aclose()
        await next_ref.release()
        next_release = await self.fixture.next_control()
        self.assertEqual(
            (next_release.WhichOneof("body"), next_release.control_id),
            ("release", 3),
        )
        self.assertEqual(next_release.release.resource_id, 59)

        self.assertEqual(
            [control.control_id for control in self.fixture.controls], [0, 1, 2, 3]
        )
        self.assertEqual(client._acked_control_id, 3)
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._pending_control_ids)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._opening_routes)
        self.assertIsNone(client._terminal_error)

    async def test_disconnect_retires_refs_routes_and_waiters(self) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        self.fixture.delay_ack(1)
        ref = client.create_resource_reference(47)
        await self.fixture.next_control()
        opening = asyncio.create_task(ref.client.open_call("test.Service", "Wait"))
        await yield_turn()
        await self.fixture.disconnect()
        with self.assertRaises(ResourceTerminalError):
            await opening
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._opening_routes)

    async def test_final_release_cancels_active_call_and_wakes_waiter(self) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        ref = client.create_resource_reference(53)
        await self.fixture.next_control()
        call = await ref.client.open_call("test.Service", "Block")
        await self.fixture.route_data.wait()
        releasing = asyncio.create_task(ref.release())
        with self.assertRaises(CallError):
            await call.receive()
        await call.aclose()
        await releasing
        await self.fixture.route_closed.wait()
        self.assertFalse(client._active_routes)

    async def test_retained_root_can_be_referenced_after_local_final_release(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        root = client.access_root_resource()
        await self.fixture.next_control()
        await root.release()
        await self.fixture.next_control()

        again = client.access_root_resource()
        adopt = await self.fixture.next_control()
        self.assertEqual((adopt.WhichOneof("body"), adopt.control_id), ("adopt", 3))
        self.assertEqual(adopt.adopt.resource_id, client.root_resource_id)
        await again.release()
        await self.fixture.next_control()

    async def test_disconnect_during_aclose_settles_request_queue_and_tasks(
        self,
    ) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        client.create_resource_reference(61)
        await self.fixture.next_control()
        self.fixture.delay_ack(2)

        closing = asyncio.create_task(client.aclose())
        release = await self.fixture.next_control()
        self.assertEqual(
            (release.WhichOneof("body"), release.control_id), ("release", 2)
        )
        await self.fixture.disconnect()
        await closing

        self.client = None
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._pending_control_ids)
        self.assertFalse(client._opening_routes)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._release_tasks)
        self.assertTrue(client._controls.empty())
        await client._controls.join()
        assert client._response_task is not None
        self.assertTrue(client._response_task.done())
        assert client._close_task is not None
        self.assertTrue(client._close_task.done())

    async def test_response_loss_between_release_join_and_enqueue_closes_cleanly(
        self,
    ) -> None:
        client = await _JoinGatedResourceClient.open(self.fixture.service)
        self.client = client
        await self.fixture.next_control()
        adopt_ack = self.fixture.delay_ack(1)
        client.create_resource_reference(67)
        await self.fixture.next_control()

        closing = asyncio.create_task(client.aclose())
        await client.join_started.wait()
        await self.fixture.disconnect()
        adopt_ack.set()
        assert client._response_task is not None
        await client._response_task

        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._pending_control_ids)
        self.assertFalse(client._opening_routes)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._release_tasks)
        self.assertTrue(client._controls.empty())
        self.assertTrue(client._response_task.done())
        self.assertFalse(closing.done())

        client.join_release.set()
        await closing

        self.client = None
        await client._controls.join()
        assert client._close_task is not None
        self.assertTrue(client._close_task.done())

    async def test_aclose_drains_sorted_releases_and_owner_state(self) -> None:
        client = await self.open_client()
        await self.fixture.next_control()
        client.create_resource_reference(29)
        client.create_resource_reference(13)
        await self.fixture.next_control()
        await self.fixture.next_control()
        await client.aclose()
        self.client = None
        releases = [
            control.release.resource_id
            for control in self.fixture.controls
            if control.WhichOneof("body") == "release"
        ]
        self.assertEqual(releases, [13, 29])
        self.assertFalse(client._resources)
        self.assertFalse(client._control_waiters)
        self.assertFalse(client._active_routes)
        self.assertFalse(client._opening_routes)
        self.assertIsNotNone(client._response_task)
        assert client._response_task is not None
        self.assertTrue(client._response_task.done())


if __name__ == "__main__":
    unittest.main()
