"""The immutable Python Resource generation client."""

from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator, AsyncIterator
from dataclasses import dataclass, field
from typing import Self, cast

from google.protobuf.message import DecodeError
from starpc.call import CallError
from starpc.client import Client
from starpc.rpcstream import build_rpc_stream_open_stream
from starpc.stream import ByteStream

from bldr.resource import resource_pb2
from bldr.resource.resource_srpc import ResourceServiceClient

from .errors import ResourceProtocolError, ResourceReleasedError, ResourceTerminalError

_QUEUE_END = object()
_MAX_CONTROL_ID = (1 << 32) - 1


def _retrieve_future_failure(future: asyncio.Future[None]) -> None:
    if not future.cancelled():
        future.exception()


@dataclass(eq=False)
class _ResourceState:
    """One resource ID's local references and lazy nested route."""

    resource_id: int
    refs: set[ResourceRef] = field(default_factory=set)
    routed_client: Client | None = None
    routes: set[ByteStream] = field(default_factory=set)
    control_ids: set[int] = field(default_factory=set)
    opening_routes: set[asyncio.Task[ByteStream]] = field(default_factory=set)
    retired: asyncio.Future[None] = field(init=False)
    terminal: bool = False

    def __post_init__(self) -> None:
        self.retired = asyncio.get_running_loop().create_future()


class ResourceRef:
    """One explicit local reference to a Resource in one generation."""

    def __init__(self, owner: ResourceClient, state: _ResourceState) -> None:
        self._owner = owner
        self._state = state
        self._released = False
        self._release_task: asyncio.Task[None] | None = None

    @property
    def id(self) -> int:
        """Return this reference's protocol resource ID."""
        return self._state.resource_id

    @property
    def released(self) -> bool:
        """Report whether this local reference can no longer be used."""
        return self._released

    @property
    def client(self) -> Client:
        """Return the lazy StarPC client routed to this Resource."""
        self._owner._require_ref_live(self)
        if self._state.routed_client is None:
            self._state.routed_client = Client(
                lambda: self._owner._open_route(self._state)
            )
        return self._state.routed_client

    def clone(self) -> ResourceRef:
        """Create another local reference to the same Resource."""
        self._owner._require_ref_live(self)
        return self._owner.create_resource_reference(self.id)

    def create_resource_reference(self, resource_id: int) -> ResourceRef:
        """Create a local reference to a child Resource ID."""
        self._owner._require_ref_live(self)
        return self._owner.create_resource_reference(resource_id)

    async def release(self) -> None:
        """Release this reference, awaiting the final Release acknowledgement."""
        if self._release_task is None:
            self._released = True
            task = asyncio.create_task(self._owner._release_reference(self))
            self._release_task = task
            self._owner._release_tasks.add(task)
            task.add_done_callback(self._owner._release_tasks.discard)
        await asyncio.shield(self._release_task)

    async def __aenter__(self) -> Self:
        self._owner._require_ref_live(self)
        return self

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> None:
        await self.release()


class ResourceClient:
    """Own one ResourceClient generation over a generated Resource service."""

    def __init__(self, service: ResourceServiceClient) -> None:
        self._service = service
        self._controls: asyncio.Queue[resource_pb2.ResourceClientRequest | object] = (
            asyncio.Queue()
        )
        self._responses: (
            AsyncGenerator[resource_pb2.ResourceClientResponse, None] | None
        ) = None
        self._response_task: asyncio.Task[None] | None = None
        self._resources: dict[int, _ResourceState] = {}
        self._control_waiters: dict[int, asyncio.Future[None]] = {}
        self._pending_control_ids: set[int] = set()
        self._active_routes: set[ByteStream] = set()
        self._opening_routes: set[asyncio.Task[ByteStream]] = set()
        self._release_tasks: set[asyncio.Task[None]] = set()
        self._control_id = 0
        self._acked_control_id = 0
        self._request_closed = False
        self._closing = False
        self._terminal_error: ResourceTerminalError | None = None
        self._close_task: asyncio.Task[None] | None = None
        self.client_handle_id = 0
        self.root_resource_id = 0

    @classmethod
    async def open(cls, service: ResourceServiceClient) -> Self:
        """Start a generation and validate its immutable server identity."""
        client = cls(service)
        client._controls.put_nowait(
            resource_pb2.ResourceClientRequest(
                control_id=0,
                init=resource_pb2.ResourceClientInitRequest(),
            )
        )
        responses = cast(
            AsyncGenerator[resource_pb2.ResourceClientResponse, None],
            service.resource_client(client._iter_controls()),
        )
        client._responses = responses
        try:
            response = await anext(responses)
            if response.WhichOneof("body") != "init":
                raise ResourceProtocolError("first ResourceClient response is not Init")
            handle_id = response.init.client_handle_id
            root_id = response.init.root_resource_id
            if handle_id == 0 or root_id == 0:
                raise ResourceProtocolError(
                    "ResourceClient Init contains an empty handle or root ID"
                )
        except asyncio.CancelledError:
            await client._close_response_iterator()
            raise
        except ResourceTerminalError as exc:
            await client._retire_generation(exc)
            await client._close_response_iterator()
            raise
        except Exception as exc:
            error = ResourceTerminalError("ResourceClient Init stream failed")
            await client._retire_generation(error)
            await client._close_response_iterator()
            raise error from exc

        client.client_handle_id = handle_id
        client.root_resource_id = root_id
        client._response_task = asyncio.create_task(client._consume_responses())
        return client

    def access_root_resource(self) -> ResourceRef:
        """Create a local reference to this generation's retained root."""
        return self.create_resource_reference(self.root_resource_id)

    def create_resource_reference(self, resource_id: int) -> ResourceRef:
        """Create a local reference and adopt a Resource on its first use."""
        self._require_generation_live()
        if resource_id == 0:
            raise ValueError("resource ID must be nonzero")
        state = self._resources.get(resource_id)
        if state is None:
            state = _ResourceState(resource_id)
            self._resources[resource_id] = state
            request = resource_pb2.ResourceClientRequest(
                adopt=resource_pb2.ResourceClientAdopt(resource_id=resource_id)
            )
            self._enqueue_control(request)
            state.control_ids.add(request.control_id)
        ref = ResourceRef(self, state)
        state.refs.add(ref)
        return ref

    async def aclose(self) -> None:
        """Release remaining resources and drain this immutable generation."""
        if self._close_task is None:
            self._close_task = asyncio.create_task(self._close())
        await asyncio.shield(self._close_task)

    async def __aenter__(self) -> Self:
        self._require_generation_live()
        return self

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> None:
        await self.aclose()

    async def _iter_controls(
        self,
    ) -> AsyncIterator[resource_pb2.ResourceClientRequest]:
        while True:
            item = await self._controls.get()
            try:
                if item is _QUEUE_END:
                    return
                assert isinstance(item, resource_pb2.ResourceClientRequest)
                yield item
            finally:
                self._controls.task_done()

    async def _consume_responses(self) -> None:
        responses = self._responses
        assert responses is not None
        try:
            async for response in responses:
                await self._handle_response(response)
                if self._terminal_error is not None:
                    return
        except asyncio.CancelledError:
            raise
        except ResourceTerminalError as exc:
            await self._retire_generation(exc)
        except (CallError, DecodeError, EOFError, OSError, ValueError):
            await self._retire_generation(
                ResourceTerminalError("ResourceClient response stream failed")
            )
        else:
            if self._terminal_error is None and (
                not self._closing or self._control_waiters
            ):
                await self._retire_generation(
                    ResourceTerminalError("ResourceClient response stream closed")
                )
        finally:
            if self._terminal_error is not None:
                await self._close_response_iterator()

    async def _handle_response(
        self, response: resource_pb2.ResourceClientResponse
    ) -> None:
        body = response.WhichOneof("body")
        if body == "control_ack":
            self._acknowledge_control(response.control_ack.control_id)
            return
        if body == "resource_released":
            resource_id = response.resource_released.resource_id
            if resource_id == 0:
                raise ResourceProtocolError("ResourceReleased has an empty resource ID")
            state = self._resources.get(resource_id)
            if state is not None:
                await self._retire_state(state)
            return
        raise ResourceProtocolError("unexpected ResourceClient response")

    def _enqueue_control(
        self, request: resource_pb2.ResourceClientRequest
    ) -> asyncio.Future[None]:
        if self._terminal_error is not None:
            raise self._terminal_error
        if self._control_id == _MAX_CONTROL_ID:
            raise ResourceProtocolError("ResourceClient control IDs are exhausted")
        self._control_id += 1
        request.control_id = self._control_id
        waiter: asyncio.Future[None] = asyncio.get_running_loop().create_future()
        waiter.add_done_callback(_retrieve_future_failure)
        self._control_waiters[request.control_id] = waiter
        self._pending_control_ids.add(request.control_id)
        self._controls.put_nowait(request)
        return waiter

    def _acknowledge_control(self, control_id: int) -> None:
        if (
            control_id == 0
            or control_id != self._acked_control_id + 1
            or control_id > self._control_id
        ):
            raise ResourceProtocolError(
                f"unexpected ResourceClient control acknowledgement {control_id}"
            )
        if control_id not in self._pending_control_ids:
            raise ResourceProtocolError(
                f"unknown ResourceClient control acknowledgement {control_id}"
            )
        self._pending_control_ids.remove(control_id)
        waiter = self._control_waiters.pop(control_id, None)
        self._acked_control_id = control_id
        if waiter is not None and not waiter.done():
            waiter.set_result(None)

    async def _release_reference(self, ref: ResourceRef) -> None:
        state = ref._state
        state.refs.discard(ref)
        if state.refs or state.terminal:
            return
        await self._retire_state(state)
        if self._terminal_error is not None:
            return
        waiter = self._enqueue_control(
            resource_pb2.ResourceClientRequest(
                release=resource_pb2.ResourceClientRelease(
                    resource_id=state.resource_id
                )
            )
        )
        await waiter

    async def _open_route(self, state: _ResourceState) -> ByteStream:
        self._require_state_live(state)
        target = self._control_id
        if target > self._acked_control_id:
            waiter = self._control_waiters.get(target)
            if waiter is None:
                raise ResourceProtocolError("missing ResourceClient control waiter")
            done, _ = await asyncio.wait(
                (waiter, state.retired), return_when=asyncio.FIRST_COMPLETED
            )
            if state.retired in done:
                self._require_state_live(state)
            await waiter
        self._require_state_live(state)

        opener = build_rpc_stream_open_stream(
            str(state.resource_id), self._service.resource_rpc
        )
        task = asyncio.current_task()
        assert task is not None
        opening = cast(asyncio.Task[ByteStream], task)
        state.opening_routes.add(opening)
        self._opening_routes.add(opening)
        try:
            self._require_state_live(state)
            stream = await opener()
            if not self._state_is_live(state):
                await stream.aclose()
                self._require_state_live(state)
            state.routes.add(stream)
            self._active_routes.add(stream)
            return stream
        except asyncio.CancelledError:
            self._require_state_live(state)
            raise
        finally:
            state.opening_routes.discard(opening)
            self._opening_routes.discard(opening)

    async def _retire_state(self, state: _ResourceState) -> None:
        if state.terminal:
            return
        state.terminal = True
        if not state.retired.done():
            state.retired.set_result(None)
        for control_id in state.control_ids:
            waiter = self._control_waiters.pop(control_id, None)
            if waiter is not None and not waiter.done():
                waiter.set_result(None)
        if self._resources.get(state.resource_id) is state:
            del self._resources[state.resource_id]
        for ref in state.refs:
            ref._released = True
        state.refs.clear()
        routes = tuple(state.routes)
        state.routes.clear()
        self._active_routes.difference_update(routes)
        for route in routes:
            await route.aclose()
        await self._cancel_opening_routes(state)

    async def _retire_generation(self, error: ResourceTerminalError) -> None:
        if self._terminal_error is not None:
            return
        self._terminal_error = error
        self._closing = True
        states = tuple(self._resources.values())
        self._resources.clear()
        for state in states:
            state.terminal = True
            if not state.retired.done():
                state.retired.set_result(None)
            for ref in state.refs:
                ref._released = True
            state.refs.clear()
        for waiter in self._control_waiters.values():
            if not waiter.done():
                waiter.set_exception(error)
        self._control_waiters.clear()
        self._pending_control_ids.clear()
        self._finish_controls(discard=True)
        for state in states:
            routes = tuple(state.routes)
            state.routes.clear()
            self._active_routes.difference_update(routes)
            for route in routes:
                await route.aclose()
            await self._cancel_opening_routes(state)

    async def _close(self) -> None:
        if self._terminal_error is not None:
            self._finish_controls(discard=True)
            await self._join_response_task()
            await self._join_release_tasks()
            self._finish_controls(discard=True)
            await self._controls.join()
            return

        self._closing = True
        states = sorted(self._resources.values(), key=lambda state: state.resource_id)
        self._resources.clear()
        for state in states:
            state.terminal = True
            if not state.retired.done():
                state.retired.set_result(None)
            for ref in state.refs:
                ref._released = True
            state.refs.clear()
        for state in states:
            routes = tuple(state.routes)
            state.routes.clear()
            self._active_routes.difference_update(routes)
            for route in routes:
                await route.aclose()
            await self._cancel_opening_routes(state)
        # A final reference may still be joining an active nested route. It
        # owns its Release, so drain it before ending the control iterator.
        await self._join_release_tasks()
        if self._terminal_error is None:
            for state in states:
                self._enqueue_control(
                    resource_pb2.ResourceClientRequest(
                        release=resource_pb2.ResourceClientRelease(
                            resource_id=state.resource_id
                        )
                    )
                )
        target = self._control_id
        if target > self._acked_control_id:
            waiter = self._control_waiters.get(target)
            if waiter is not None:
                try:
                    await waiter
                except ResourceTerminalError:
                    pass
        self._finish_controls()
        await self._controls.join()
        await self._join_response_task()
        await self._join_release_tasks()
        if self._terminal_error is None:
            self._terminal_error = ResourceTerminalError("ResourceClient closed")

    def _finish_controls(self, *, discard: bool = False) -> None:
        if not self._request_closed:
            self._request_closed = True
            self._controls.put_nowait(_QUEUE_END)
        if discard:
            while True:
                try:
                    self._controls.get_nowait()
                except asyncio.QueueEmpty:
                    return
                self._controls.task_done()

    async def _close_response_iterator(self) -> None:
        responses = self._responses
        if responses is not None:
            await responses.aclose()

    async def _join_response_task(self) -> None:
        task = self._response_task
        if task is not None and task is not asyncio.current_task():
            await task

    async def _cancel_opening_routes(self, state: _ResourceState) -> None:
        current = asyncio.current_task()
        tasks = tuple(task for task in state.opening_routes if task is not current)
        for task in tasks:
            if not task.done():
                task.cancel()
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

    async def _join_release_tasks(self) -> None:
        current = asyncio.current_task()
        tasks = tuple(task for task in self._release_tasks if task is not current)
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

    def _require_generation_live(self) -> None:
        if self._terminal_error is not None:
            raise self._terminal_error
        if self._closing:
            raise ResourceTerminalError("ResourceClient is closing")

    def _require_ref_live(self, ref: ResourceRef) -> None:
        if ref._released:
            raise ResourceReleasedError(f"resource {ref.id} was released")
        self._require_state_live(ref._state)

    def _require_state_live(self, state: _ResourceState) -> None:
        self._require_generation_live()
        if not self._state_is_live(state):
            raise ResourceReleasedError(f"resource {state.resource_id} was released")

    def _state_is_live(self, state: _ResourceState) -> bool:
        return not state.terminal and self._resources.get(state.resource_id) is state


__all__ = ["ResourceClient", "ResourceRef"]
