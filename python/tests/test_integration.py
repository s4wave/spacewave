from __future__ import annotations

import contextlib
import unittest

from _resource_fixture import ResourceServerHarness
from starpc.call import Call
from starpc.server import ServiceRegistry

from spacewave_resource import ResourceCall, ResourceClient

_SERVICE = "test.Integration"


class ResourceIntegrationTest(unittest.IsolatedAsyncioTestCase):
    async def test_client_routes_through_server_and_settles_all_owners(self) -> None:
        releases: list[int] = []

        def factory(registry: ServiceRegistry, resource_call: ResourceCall) -> None:
            async def echo(call: Call) -> None:
                data = await call.receive()
                assert data is not None
                await call.send(data)

            registry.register(_SERVICE, "Echo", echo)

        harness = ResourceServerHarness(factory)
        client = await ResourceClient.open(harness.service)
        try:
            root = client.access_root_resource()
            call = await root.client.open_call(_SERVICE, "Echo")
            await call.send(b"joined")
            await call.finish()
            self.assertEqual(await call.receive(), b"joined")
            self.assertIsNone(await call.receive())
            await call.aclose()
            await root.release()
            await client.aclose()
            self.assertFalse(client._resources)
            self.assertFalse(client._active_routes)
            self.assertFalse(harness.server._generations)
            self.assertEqual(releases, [])
        finally:
            with contextlib.suppress(Exception):
                await client.aclose()
            await harness.aclose()


if __name__ == "__main__":
    unittest.main()
