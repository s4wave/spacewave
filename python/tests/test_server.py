from __future__ import annotations

import asyncio
import contextlib
import unittest
from collections import defaultdict
from collections.abc import AsyncIterator, Awaitable, Callable

from _resource_fixture import ResourceControlStream, ResourceServerHarness
from starpc.call import Call, CallError
from starpc.client import Client
from starpc.rpcstream import RpcStreamRemoteError, build_rpc_stream_open_stream
from starpc.server import Handler, ServiceRegistry
from starpc.stream import StreamClosedError

from bldr.resource import resource_pb2
from spacewave_resource import ResourceCall, ResourceUnsupportedError

_ALPHA = "test.Alpha"
_BETA = "test.Beta"


class ReleaseRecord:
    """One release callback that reports its resource ID when the server runs it."""

    def __init__(
        self,
        order: list[str],
        awaited: bool = False,
        gate: asyncio.Event | None = None,
        entered: asyncio.Event | None = None,
        fail: bool = False,
    ) -> None:
        self.resource_id = 0
        self._order = order
        self._awaited = awaited
        self._gate = gate
        self._entered = entered
        self._fail = fail

    def __call__(self) -> Awaitable[None] | None:
        if self._awaited:
            return self._record_awaited()
        self._record()
        return None

    async def _record_awaited(self) -> None:
        if self._entered is not None:
            self._entered.set()
        if self._gate is not None:
            await self._gate.wait()
        self._record()

    def _record(self) -> None:
        self._order.append(f"release:{self.resource_id}")
        if self._fail:
            raise ValueError(f"release callback failed for {self.resource_id}")


class LifecycleProbe:
    """Collect handler, callback, and provenance events without sleeps."""

    def __init__(self) -> None:
        self.order: list[str] = []
        self.provenance: dict[int, tuple[int, int, str, str]] = {}
        self.releasers: dict[int, Callable[[], Awaitable[None]]] = {}
        self.gate = asyncio.Event()
        self.gate.set()
        self.release_gate = asyncio.Event()
        self.release_gate.set()
        self._events: defaultdict[str, asyncio.Event] = defaultdict(asyncio.Event)

    def event(self, name: str) -> asyncio.Event:
        return self._events[name]


class RecordingResource:
    """One test resource whose handlers report provenance and release order."""

    def __init__(self, probe: LifecycleProbe) -> None:
        self._probe = probe

    def factory(self, registry: ServiceRegistry, call: ResourceCall) -> None:
        for service in (_ALPHA, _BETA):
            registry.register(service, "Echo", self._echo)
            registry.register(service, "Spawn", self._spawn(call, awaited=False))
            registry.register(service, "SpawnAwait", self._spawn(call, awaited=True))
            registry.register(
                service, "SpawnFail", self._spawn(call, awaited=True, fail=True)
            )
            registry.register(service, "SpawnRelease", self._spawn_release(call))
            registry.register(
                service, "SpawnReleaseFail", self._spawn_release(call, fail=True)
            )
            registry.register(service, "SelfRelease", self._self_release(call))
            registry.register(service, "Block", self._block(call))

    async def _echo(self, call: Call) -> None:
        while True:
            data = await call.receive()
            if data is None:
                return
            await call.send(data)

    def _spawn(self, call: ResourceCall, awaited: bool, fail: bool = False) -> Handler:
        async def handler(rpc: Call) -> None:
            await rpc.receive()
            self._probe.event(f"enter:{call.service}.{call.method}").set()
            await self._probe.gate.wait()
            record = ReleaseRecord(
                self._probe.order,
                awaited,
                self._probe.release_gate,
                self._probe.event("callback-entered"),
                fail,
            )
            child = await call.construct_child_resource(self.factory, record)
            record.resource_id = child.id
            self._probe.releasers[child.id] = child.release_resource
            self._probe.provenance[child.id] = (
                call.generation,
                call.parent,
                call.service,
                call.method,
            )
            await rpc.send(str(child.id).encode())

        return handler

    def _spawn_release(self, call: ResourceCall, fail: bool = False) -> Handler:
        async def handler(rpc: Call) -> None:
            await rpc.receive()
            record = ReleaseRecord(self._probe.order, awaited=True, fail=fail)
            child = await call.construct_child_resource(self.factory, record)
            record.resource_id = child.id
            await child.release_resource()
            await rpc.send(str(child.id).encode())

        return handler

    def _self_release(self, call: ResourceCall) -> Handler:
        async def handler(rpc: Call) -> None:
            await rpc.receive()
            await self._probe.releasers[call.parent]()
            self._probe.order.append(f"self-continued:{call.parent}")

        return handler

    def _block(self, call: ResourceCall) -> Handler:
        async def handler(rpc: Call) -> None:
            self._probe.event(f"block:{call.parent}").set()
            try:
                while await rpc.receive() is not None:
                    pass
            finally:
                self._probe.order.append(f"handler:{call.parent}")

        return handler


async def _receive_id(call: Call) -> int:
    data = await call.receive()
    assert data is not None
    return int(data.decode())


async def _round_trip(
    client: Client, service: str, method: str, request: bytes
) -> bytes:
    """Run one complete nested call and prove the route completed."""
    call = await client.open_call(service, method)
    await call.send(request)
    await call.finish()
    response = await call.receive()
    assert response is not None
    assert await call.receive() is None
    await call.aclose()
    return response


class ResourceServerTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.probe = LifecycleProbe()
        self.resource = RecordingResource(self.probe)
        self.harness = ResourceServerHarness(self.resource.factory)
        self.controls: list[ResourceControlStream] = []

    async def asyncTearDown(self) -> None:
        for control in self.controls:
            with contextlib.suppress(CallError, StreamClosedError):
                await control.aclose()
        await self.harness.aclose()
        self.assertFalse(self.harness.server._generations)
        self.assertFalse(self.harness.server._handler_routes)

    async def open_control(self) -> ResourceControlStream:
        control = ResourceControlStream(self.harness.service)
        self.controls.append(control)
        await control.open()
        return control

    async def spawn_child(
        self, resource_id: int, service: str = _ALPHA, method: str = "Spawn"
    ) -> int:
        client = self.harness.open_route_client(resource_id)
        call = await client.open_call(service, method)
        await call.send(b"")
        await call.finish()
        child_id = await _receive_id(call)
        self.assertIsNone(await call.receive())
        await call.aclose()
        return child_id

    async def test_init_is_required_first_and_identity_is_unique(self) -> None:
        first = await self.open_control()
        second = await self.open_control()
        self.assertNotEqual(first.client_handle_id, 0)
        self.assertNotEqual(first.root_resource_id, 0)
        self.assertNotEqual(first.client_handle_id, second.client_handle_id)
        self.assertNotEqual(first.root_resource_id, second.root_resource_id)
        self.assertEqual(
            sorted(self.harness.server._generations),
            [first.client_handle_id, second.client_handle_id],
        )

    async def test_non_init_first_control_fails_the_generation(self) -> None:
        control = ResourceControlStream(self.harness.service)
        control.adopt(1, 1)
        with self.assertRaises(CallError):
            await control.receive()
        self.assertFalse(self.harness.server._generations)

    async def test_out_of_order_control_fails_the_generation(self) -> None:
        control = await self.open_control()
        control.adopt(2, control.root_resource_id)
        with self.assertRaises(CallError):
            await control.receive()
        self.assertFalse(self.harness.server._generations)

    async def test_adopt_of_unknown_resource_fails_the_generation(self) -> None:
        control = await self.open_control()
        control.adopt(1, control.root_resource_id + 4242)
        with self.assertRaises(CallError):
            await control.receive()
        self.assertFalse(self.harness.server._generations)

    async def test_retained_root_release_frees_pending_descendants_child_first(
        self,
    ) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root)
        grandchild = await self.spawn_child(child)

        control.release(1, root)
        first = await control.receive()
        second = await control.receive()
        ack = await control.receive()
        self.assertEqual(
            (first.WhichOneof("body"), first.resource_released.resource_id),
            ("resource_released", grandchild),
        )
        self.assertEqual(
            (second.WhichOneof("body"), second.resource_released.resource_id),
            ("resource_released", child),
        )
        self.assertEqual(
            (ack.WhichOneof("body"), ack.control_ack.control_id), ("control_ack", 1)
        )
        self.assertEqual(
            self.probe.order, [f"release:{grandchild}", f"release:{child}"]
        )
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertEqual(sorted(generation.resources), [root])

    async def test_retained_root_stays_routable_after_release(self) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        control.release(1, root)
        ack = await control.receive()
        self.assertEqual(ack.control_ack.control_id, 1)

        routed = self.harness.open_route_client(root)
        self.assertEqual(await _round_trip(routed, _ALPHA, "Echo", b"live"), b"live")
        control.adopt(2, root)
        self.assertEqual((await control.receive()).control_ack.control_id, 2)

        await control.aclose()
        self.assertFalse(self.harness.server._generations)

    async def test_adopted_child_survives_parent_release(self) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root)
        control.adopt(1, child)
        self.assertEqual((await control.receive()).control_ack.control_id, 1)

        control.release(2, root)
        ack = await control.receive()
        self.assertEqual(
            (ack.WhichOneof("body"), ack.control_ack.control_id), ("control_ack", 2)
        )
        self.assertEqual(self.probe.order, [])
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertEqual(sorted(generation.resources), sorted([root, child]))
        routed = self.harness.open_route_client(child)
        self.assertEqual(await _round_trip(routed, _ALPHA, "Echo", b"alive"), b"alive")

    async def test_late_adopt_of_released_child_notifies_before_ack(self) -> None:
        control = await self.open_control()
        child = await self.spawn_child(control.root_resource_id)
        control.adopt(1, child)
        self.assertEqual((await control.receive()).control_ack.control_id, 1)
        control.release(2, child)
        ack = await control.receive()
        self.assertEqual(
            (ack.WhichOneof("body"), ack.control_ack.control_id), ("control_ack", 2)
        )

        control.adopt(3, child)
        released = await control.receive()
        late_ack = await control.receive()
        self.assertEqual(
            (released.WhichOneof("body"), released.resource_released.resource_id),
            ("resource_released", child),
        )
        self.assertEqual(
            (late_ack.WhichOneof("body"), late_ack.control_ack.control_id),
            ("control_ack", 3),
        )
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertNotIn(child, generation.resources)
        opener = build_rpc_stream_open_stream(
            str(child), self.harness.service.resource_rpc
        )
        with self.assertRaises(RpcStreamRemoteError):
            await opener()

    async def test_concurrent_routes_bind_exact_provenance(self) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root)
        self.probe.gate.clear()

        alpha = await self.harness.open_route_client(root).open_call(_ALPHA, "Spawn")
        beta = await self.harness.open_route_client(child).open_call(
            _BETA, "SpawnAwait"
        )
        await alpha.send(b"")
        await alpha.finish()
        await beta.send(b"")
        await beta.finish()
        await self.probe.event(f"enter:{_ALPHA}.Spawn").wait()
        await self.probe.event(f"enter:{_BETA}.SpawnAwait").wait()
        self.probe.gate.set()

        alpha_id = await _receive_id(alpha)
        beta_id = await _receive_id(beta)
        handle = control.client_handle_id
        self.assertEqual(
            self.probe.provenance[alpha_id], (handle, root, _ALPHA, "Spawn")
        )
        self.assertEqual(
            self.probe.provenance[beta_id], (handle, child, _BETA, "SpawnAwait")
        )
        await alpha.aclose()
        await beta.aclose()

    async def test_release_joins_active_handler_before_callback_and_ack(self) -> None:
        control = await self.open_control()
        child = await self.spawn_child(control.root_resource_id, method="SpawnAwait")
        blocked = await self.harness.open_route_client(child).open_call(_ALPHA, "Block")
        await self.probe.event(f"block:{child}").wait()
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertEqual(len(generation.resources[child].routes), 1)

        self.probe.release_gate.clear()
        control.release(1, child)
        ack_waiter = asyncio.create_task(control.receive())
        await self.probe.event("callback-entered").wait()
        self.assertFalse(ack_waiter.done())
        self.probe.release_gate.set()
        ack = await ack_waiter
        self.assertEqual(
            (ack.WhichOneof("body"), ack.control_ack.control_id), ("control_ack", 1)
        )
        self.assertEqual(self.probe.order, [f"handler:{child}", f"release:{child}"])
        self.assertNotIn(child, generation.resources)
        with self.assertRaises(CallError):
            await blocked.receive()
        await blocked.aclose()

    async def test_dropping_one_resource_rpc_leaves_the_resource_usable(self) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        dropped = await self.harness.open_route_client(root).open_call(_ALPHA, "Block")
        dropped_task = self.harness.tasks[-1]
        await self.probe.event(f"block:{root}").wait()
        survivor = await self.harness.open_route_client(root).open_call(_ALPHA, "Echo")
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertEqual(len(generation.resources[root].routes), 2)

        await dropped.aclose()
        await dropped_task
        self.assertEqual(len(generation.resources[root].routes), 1)

        await survivor.send(b"live")
        self.assertEqual(await survivor.receive(), b"live")
        await survivor.finish()
        self.assertIsNone(await survivor.receive())
        await survivor.aclose()
        routed = self.harness.open_route_client(root)
        self.assertEqual(await _round_trip(routed, _ALPHA, "Echo", b"again"), b"again")

    async def test_server_release_removes_state_before_callback_and_notification(
        self,
    ) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root, method="SpawnRelease")
        generation = self.harness.server._generations[control.client_handle_id]
        self.assertNotIn(child, generation.resources)
        self.assertEqual(self.probe.order, [f"release:{child}"])

        released = await control.receive()
        self.assertEqual(
            (released.WhichOneof("body"), released.resource_released.resource_id),
            ("resource_released", child),
        )
        routed = self.harness.open_route_client(root)
        self.assertEqual(await _round_trip(routed, _ALPHA, "Echo", b"live"), b"live")

    async def test_handler_self_release_settles_others_then_terminates_current(
        self,
    ) -> None:
        control = await self.open_control()
        child = await self.spawn_child(control.root_resource_id, method="SpawnAwait")
        blocked = await self.harness.open_route_client(child).open_call(_ALPHA, "Block")
        await self.probe.event(f"block:{child}").wait()

        self_call = await self.harness.open_route_client(child).open_call(
            _ALPHA, "SelfRelease"
        )
        await self_call.send(b"")
        await self_call.finish()
        released = await control.receive()

        generation = self.harness.server._generations[control.client_handle_id]
        self.assertNotIn(child, generation.resources)
        self.assertEqual(
            (released.WhichOneof("body"), released.resource_released.resource_id),
            ("resource_released", child),
        )
        self.assertEqual(self.probe.order, [f"handler:{child}", f"release:{child}"])
        self.assertNotIn(f"self-continued:{child}", self.probe.order)
        with self.assertRaises(CallError):
            await blocked.receive()
        await blocked.aclose()
        with self.assertRaises(CallError):
            await self_call.receive()
        await self_call.aclose()

        opener = build_rpc_stream_open_stream(
            str(child), self.harness.service.resource_rpc
        )
        with self.assertRaises(RpcStreamRemoteError):
            await opener()
        self.assertNotIn(f"self-continued:{child}", self.probe.order)
        self.assertFalse(self.harness.server._handler_routes)

    async def test_server_release_reports_callback_failure_and_notifies(self) -> None:
        loop = asyncio.get_running_loop()
        reported: list[dict[str, object]] = []
        previous = loop.get_exception_handler()
        loop.set_exception_handler(lambda _loop, context: reported.append(context))
        self.addCleanup(loop.set_exception_handler, previous)

        control = await self.open_control()
        child = await self.spawn_child(
            control.root_resource_id, method="SpawnReleaseFail"
        )
        released = await control.receive()

        self.assertEqual(
            (released.WhichOneof("body"), released.resource_released.resource_id),
            ("resource_released", child),
        )
        self.assertEqual(self.probe.order, [f"release:{child}"])
        self.assertEqual(len(reported), 1)
        self.assertEqual(reported[0]["resource_id"], child)
        self.assertIsInstance(reported[0]["exception"], ValueError)

    async def test_self_release_reports_callback_failure_and_stays_terminal(
        self,
    ) -> None:
        loop = asyncio.get_running_loop()
        reported: list[dict[str, object]] = []
        previous = loop.get_exception_handler()
        loop.set_exception_handler(lambda _loop, context: reported.append(context))
        self.addCleanup(loop.set_exception_handler, previous)

        control = await self.open_control()
        child = await self.spawn_child(control.root_resource_id, method="SpawnFail")
        self_call = await self.harness.open_route_client(child).open_call(
            _ALPHA, "SelfRelease"
        )
        await self_call.send(b"")
        await self_call.finish()
        with self.assertRaises(CallError):
            await self_call.receive()
        await self_call.aclose()

        self.assertEqual(len(reported), 1)
        self.assertEqual(reported[0]["resource_id"], child)
        released = await control.receive()
        self.assertEqual(released.resource_released.resource_id, child)
        self.assertNotIn(f"self-continued:{child}", self.probe.order)

    async def test_aclose_reports_each_callback_failure_and_cleans_generations(
        self,
    ) -> None:
        loop = asyncio.get_running_loop()
        reported: list[dict[str, object]] = []
        previous = loop.get_exception_handler()
        loop.set_exception_handler(lambda _loop, context: reported.append(context))
        self.addCleanup(loop.set_exception_handler, previous)

        first = await self.open_control()
        second = await self.open_control()
        first_child = await self.spawn_child(first.root_resource_id, method="SpawnFail")
        second_child = await self.spawn_child(
            second.root_resource_id, method="SpawnFail"
        )
        first.adopt(1, first_child)
        second.adopt(1, second_child)
        self.assertEqual((await first.receive()).control_ack.control_id, 1)
        self.assertEqual((await second.receive()).control_ack.control_id, 1)

        await self.harness.server.aclose()

        self.assertFalse(self.harness.server._generations)
        self.assertEqual(
            sorted(context["resource_id"] for context in reported),
            sorted([first_child, second_child]),
        )
        self.assertEqual(len(reported), 2)
        for control in (first, second):
            with self.assertRaises(StopAsyncIteration):
                await control.receive()

    async def test_disconnect_releases_retained_root_and_adopted_tree_once(
        self,
    ) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root)
        control.adopt(1, child)
        self.assertEqual((await control.receive()).control_ack.control_id, 1)
        grandchild = await self.spawn_child(child)
        control.adopt(2, grandchild)
        self.assertEqual((await control.receive()).control_ack.control_id, 2)

        await control.aclose()
        self.assertEqual(
            self.probe.order, [f"release:{grandchild}", f"release:{child}"]
        )
        self.assertFalse(self.harness.server._generations)

        await self.harness.server.aclose()
        self.assertEqual(
            self.probe.order, [f"release:{grandchild}", f"release:{child}"]
        )

    async def test_server_invalidation_cancels_routes_and_releases_tree(self) -> None:
        control = await self.open_control()
        root = control.root_resource_id
        child = await self.spawn_child(root)
        control.adopt(1, child)
        self.assertEqual((await control.receive()).control_ack.control_id, 1)
        blocked = await self.harness.open_route_client(child).open_call(_ALPHA, "Block")
        await self.probe.event(f"block:{child}").wait()

        await self.harness.server.aclose()
        self.assertEqual(self.probe.order, [f"handler:{child}", f"release:{child}"])
        self.assertFalse(self.harness.server._generations)
        with self.assertRaises(CallError):
            await blocked.receive()
        await blocked.aclose()
        with self.assertRaises(StopAsyncIteration):
            await control.receive()

    async def test_resource_attach_is_unsupported_and_retains_no_state(self) -> None:
        async def requests() -> AsyncIterator[resource_pb2.ResourceAttachRequest]:
            yield resource_pb2.ResourceAttachRequest(
                init=resource_pb2.ResourceAttachInit(client_handle_id=1)
            )

        direct = self.harness.server.resource_attach(requests())
        with self.assertRaises(ResourceUnsupportedError):
            await anext(direct)

        control = await self.open_control()
        wire = self.harness.service.resource_attach(requests())
        with self.assertRaises(CallError):
            await anext(wire)
        self.assertEqual(
            sorted(self.harness.server._generations), [control.client_handle_id]
        )

    async def test_invalid_component_id_fails_only_that_route(self) -> None:
        control = await self.open_control()
        for component_id in ("0", "root"):
            opener = build_rpc_stream_open_stream(
                component_id, self.harness.service.resource_rpc
            )
            with self.assertRaises(CallError):
                await opener()

        unknown = build_rpc_stream_open_stream(
            str(control.root_resource_id + 4242), self.harness.service.resource_rpc
        )
        with self.assertRaises(RpcStreamRemoteError):
            await unknown()

        routed = self.harness.open_route_client(control.root_resource_id)
        self.assertEqual(await _round_trip(routed, _ALPHA, "Echo", b"live"), b"live")


if __name__ == "__main__":
    unittest.main()
