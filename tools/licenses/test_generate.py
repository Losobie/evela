"""Regression coverage for the MSYS2 native-license package layouts."""

from pathlib import Path
import tempfile
import unittest

import generate


class NativeLicenseTests(unittest.TestCase):
    def test_only_in_repository_parser_replacement_is_accepted(self):
        path, version = "github.com/Losobie/evela/parser", "v0.0.0"
        module = {"Dir": str(generate.ROOT / "parser"), "Version": version, "Replace": {"Path": "./parser"}}
        self.assertEqual(generate.module_directory(path, version, module), generate.ROOT / "parser")
        with self.assertRaises(RuntimeError):
            generate.module_directory("example.com/unreviewed", version, module)
        module["Dir"] = str(generate.ROOT / "somewhere-else")
        with self.assertRaises(RuntimeError):
            generate.module_directory(path, version, module)

    def test_installed_layouts_collect_every_notice(self):
        for gcc_directory in ("gcc-libs", "libgcc"):
            with self.subTest(gcc_directory=gcc_directory), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                files = {
                    "MinGW-w64-runtime.txt": "crt/COPYING.MinGW-w64-runtime.txt",
                    "GCC-Runtime-Exception-3.1.txt": f"{gcc_directory}/COPYING.RUNTIME",
                    "GPL-3.0.txt": f"{gcc_directory}/COPYING3",
                    "Winpthreads.txt": "libwinpthread/COPYING",
                }
                for name, relative in files.items():
                    file = root / relative
                    file.parent.mkdir(parents=True, exist_ok=True)
                    file.write_text(name, encoding="utf-8")
                for name, relative in files.items():
                    selected = generate.native_license_file(name, root)
                    self.assertEqual(selected, root / relative)
                    self.assertEqual(generate.read(selected), name + "\n")

    def test_current_package_takes_priority_over_legacy_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for package in ("gcc-libs", "libgcc"):
                file = root / package / "COPYING.RUNTIME"
                file.parent.mkdir(parents=True)
                file.write_text(package, encoding="utf-8")
            selected = generate.native_license_file("GCC-Runtime-Exception-3.1.txt", root)
            self.assertEqual(generate.read(selected), "libgcc\n")

    def test_missing_installed_notice_fails_without_using_snapshot(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(RuntimeError) as raised:
                generate.native_license_file("GCC-Runtime-Exception-3.1.txt", root)
            message = str(raised.exception)
            self.assertIn(str(root / "libgcc/COPYING.RUNTIME"), message)
            self.assertIn(str(root / "gcc-libs/COPYING.RUNTIME"), message)
            self.assertIn("Review the installed MinGW package layout", message)

    def test_default_uses_reviewed_snapshots(self):
        for name in generate.NATIVE_FILES:
            with self.subTest(name=name):
                file = generate.native_license_file(name, None)
                self.assertEqual(file, generate.ROOT / "licenses/native" / name)
                self.assertTrue(generate.read(file).strip())


if __name__ == "__main__":
    unittest.main()
