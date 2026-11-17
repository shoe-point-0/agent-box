from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BINARIES = ("abox", "aboxd")


class InstallTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.temporary = tempfile.TemporaryDirectory(prefix="abox-install-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.root = Path(cls.temporary.name)
        cls.build_dir = cls.root / "build output"
        # Exercise the real build dependency once; subsequent cases reuse it.
        cls.initial_dir = cls.root / "initial install"
        cls.run_make("install", f"INSTALL_DIR={cls.initial_dir}")

    @classmethod
    def run_make(cls, *arguments: str, expect_success: bool = True) -> subprocess.CompletedProcess[str]:
        env = os.environ.copy()
        for key in ("MAKEFLAGS", "MFLAGS", "MAKEOVERRIDES", "INSTALL_DIR"):
            env.pop(key, None)
        result = subprocess.run(
            ["make", "--no-print-directory", f"BIN_DIR={cls.build_dir}",
             "VERSION=install-test", "COMMIT=test", "BUILD_DATE=test", *arguments],
            cwd=ROOT, env=env, text=True, capture_output=True, timeout=180,
        )
        if expect_success and result.returncode:
            raise AssertionError(f"make failed:\n{result.stdout}\n{result.stderr}")
        if not expect_success and result.returncode == 0:
            raise AssertionError("make unexpectedly succeeded")
        return result

    def assert_installed(self, directory: Path) -> None:
        self.assertEqual(sorted(p.name for p in directory.iterdir()), sorted(BINARIES))
        for name in BINARIES:
            with self.subTest(binary=name):
                binary = directory / name
                self.assertEqual(binary.stat().st_mode & 0o777, 0o755)
                self.assertEqual(binary.read_bytes(), (self.build_dir / name).read_bytes())
                result = subprocess.run(
                    [str(binary), "--version"], check=True, text=True,
                    capture_output=True, timeout=10,
                )
                self.assertEqual(
                    result.stdout.strip(),
                    f"{name} install-test (commit test, built test)",
                )

    def test_install_builds_both_executables(self) -> None:
        self.assert_installed(self.initial_dir)

    def test_installed_cli_initializes_another_project(self) -> None:
        project = self.root / "another project"
        project.mkdir()
        binary = self.initial_dir / "abox"
        subprocess.run(
            [str(binary), "init"], cwd=project, check=True,
            capture_output=True, text=True, timeout=10,
        )
        manifest = project / ".abox" / "box.yaml"
        self.assertTrue(manifest.is_file())
        original = manifest.read_bytes()
        self.assertTrue(original)
        # A second invocation must not silently replace project configuration.
        result = subprocess.run(
            [str(binary), "init"], cwd=project,
            capture_output=True, text=True, timeout=10,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(manifest.read_bytes(), original)

    def test_default_is_user_local(self) -> None:
        home = self.root / "home with spaces"
        self.run_make("-o", "build", "install", f"HOME={home}")
        self.assert_installed(home / ".local" / "bin")

    def test_override_and_reinstall_replace_inodes(self) -> None:
        destination = self.root / "custom bin"
        self.run_make("-o", "build", "install", f"INSTALL_DIR={destination}")
        # Open handles pin the old inodes, just as a running executable does.
        # In-place truncation would change the old contents and retain the inode.
        for name in BINARIES:
            (destination / name).write_bytes(b"previous executable\n")
        handles = [(destination / name).open("rb") for name in BINARIES]
        try:
            self.run_make("-o", "build", "install", f"INSTALL_DIR={destination}")
            for name, handle in zip(BINARIES, handles):
                self.assertNotEqual(os.fstat(handle.fileno()).st_ino,
                                    (destination / name).stat().st_ino)
                self.assertEqual(handle.read(), b"previous executable\n")
            self.assert_installed(destination)
        finally:
            for handle in handles:
                handle.close()

    def test_directory_conflict_preserves_existing_installation(self) -> None:
        destination = self.root / "conflicting install"
        destination.mkdir()
        (destination / "abox").write_bytes(b"previous executable\n")
        (destination / "aboxd").mkdir()
        self.run_make("-o", "build", "install", f"INSTALL_DIR={destination}",
                      expect_success=False)
        self.assertEqual((destination / "abox").read_bytes(), b"previous executable\n")
        self.assertTrue((destination / "aboxd").is_dir())
        self.assertEqual(sorted(path.name for path in destination.iterdir()), sorted(BINARIES))


if __name__ == "__main__":
    unittest.main()
