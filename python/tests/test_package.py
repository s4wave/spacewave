from __future__ import annotations

import importlib
import unittest


class PackageContractTest(unittest.TestCase):
    def test_public_package_imports(self) -> None:
        try:
            package = importlib.import_module("spacewave_resource")
        except ModuleNotFoundError as exc:
            self.fail(f"spacewave_resource package is absent: {exc}")
        for name in (
            "Resource",
            "ResourceCall",
            "ResourceClient",
            "ResourceServer",
            "Root",
            "Session",
        ):
            self.assertTrue(hasattr(package, name), f"{name} is absent")

    def test_generated_resource_modules_import(self) -> None:
        try:
            messages = importlib.import_module("bldr.resource.resource_pb2")
            services = importlib.import_module("bldr.resource.resource_srpc")
        except ModuleNotFoundError as exc:
            self.fail(f"generated Resource modules are absent: {exc}")
        self.assertTrue(hasattr(messages, "ResourceClientRequest"))
        self.assertTrue(hasattr(services, "ResourceServiceClient"))

    def test_generated_session_journey_modules_import(self) -> None:
        try:
            root_messages = importlib.import_module("sdk.root.root_pb2")
            root_services = importlib.import_module("sdk.root.root_srpc")
            session_messages = importlib.import_module("sdk.session.session_pb2")
            session_services = importlib.import_module("sdk.session.session_srpc")
            spaces = importlib.import_module("core.space.space_pb2")
        except ModuleNotFoundError as exc:
            self.fail(f"generated Session journey modules are absent: {exc}")
        self.assertTrue(hasattr(root_messages, "MountSessionByIdxRequest"))
        self.assertTrue(hasattr(root_services, "RootResourceServiceClient"))
        self.assertTrue(hasattr(session_messages, "WatchResourcesListRequest"))
        self.assertTrue(hasattr(session_services, "SessionResourceServiceClient"))
        self.assertTrue(hasattr(spaces, "SpaceSoListEntry"))


if __name__ == "__main__":
    unittest.main()
