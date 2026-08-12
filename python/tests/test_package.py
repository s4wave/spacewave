from __future__ import annotations

import importlib
import unittest


class PackageContractTest(unittest.TestCase):
    def test_public_package_imports(self) -> None:
        try:
            package = importlib.import_module("spacewave_resource")
        except ModuleNotFoundError as exc:
            self.fail(f"spacewave_resource package is absent: {exc}")
        self.assertTrue(hasattr(package, "ResourceClient"))
        self.assertTrue(hasattr(package, "ResourceServer"))
        self.assertTrue(hasattr(package, "ResourceCall"))

    def test_generated_resource_modules_import(self) -> None:
        try:
            messages = importlib.import_module("bldr.resource.resource_pb2")
            services = importlib.import_module("bldr.resource.resource_srpc")
        except ModuleNotFoundError as exc:
            self.fail(f"generated Resource modules are absent: {exc}")
        self.assertTrue(hasattr(messages, "ResourceClientRequest"))
        self.assertTrue(hasattr(services, "ResourceServiceClient"))


if __name__ == "__main__":
    unittest.main()
