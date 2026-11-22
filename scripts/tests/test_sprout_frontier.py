from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

MODULE_PATH = Path(__file__).resolve().parents[1] / "sprout_frontier.py"
SPEC = importlib.util.spec_from_file_location("sprout_frontier", MODULE_PATH)
assert SPEC and SPEC.loader
sprout_frontier = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = sprout_frontier
SPEC.loader.exec_module(sprout_frontier)


class FrontierTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name)
        (self.root / ".sprout" / "tasks").mkdir(parents=True)
        (self.root / "docs" / "features").mkdir(parents=True)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def write_task(self, task_id: str, dependencies: tuple[str, ...] = (), context: str = "") -> None:
        dependency_text = ""
        if dependencies:
            dependency_text = "Dependencies: " + ", ".join(dependencies) + ". "
        content = (
            f"# {task_id} title\n\n"
            "## Context\n\n"
            f"- {dependency_text}{context}\n\n"
            "## Requirements\n\n- requirement\n"
        )
        (self.root / ".sprout" / "tasks" / f"{task_id}.md").write_text(content, encoding="utf-8")

    def implement(self, task_id: str) -> None:
        (self.root / "docs" / "features" / f"{task_id}.md").write_text(
            f"# {task_id}\n", encoding="utf-8"
        )

    def test_classifies_ready_blocked_and_deferred_closure(self) -> None:
        self.implement("m0-done")
        self.write_task("m0-ready", ("m0-done",))
        self.write_task("m0-blocked", ("m0-ready",))
        self.write_task(
            "m0-darwin",
            ("m0-done",),
            "Final Darwin hardware evidence input.",
        )
        self.write_task("m1-deferred-blocked", ("m0-darwin",))
        self.write_task("m6-future", ("m1-deferred-blocked",))

        frontier = sprout_frontier.load_frontier(self.root)

        self.assertEqual([row["id"] for row in frontier["ready"]], ["m0-ready"])
        self.assertEqual([row["id"] for row in frontier["blocked"]], ["m0-blocked"])
        self.assertEqual(
            [row["id"] for row in frontier["deferred"]],
            ["m0-darwin", "m1-deferred-blocked", "m6-future"],
        )
        self.assertEqual(frontier["missingDependencies"], [])
        self.assertEqual(frontier["cycles"], [])

    def test_reports_missing_dependency(self) -> None:
        self.write_task("m0-task", ("m0-missing",))

        frontier = sprout_frontier.load_frontier(self.root)

        self.assertEqual(frontier["missingDependencies"], ["m0-missing"])

    def test_reports_dependency_cycle(self) -> None:
        self.write_task("m0-a", ("m0-b",))
        self.write_task("m0-b", ("m0-a",))

        frontier = sprout_frontier.load_frontier(self.root)

        self.assertTrue(frontier["cycles"])
        self.assertEqual(frontier["cycles"][0][0], frontier["cycles"][0][-1])

    def test_reads_implemented_task_id_from_sprout_metadata(self) -> None:
        (self.root / "docs" / "features" / "abox-up.md").write_text(
            '<!-- sprout-task\n{"schema_version":1,"id":"m1-single-repo-up","status":"implemented"}\n-->\n\n# abox up\n',
            encoding="utf-8",
        )
        self.write_task("m1-core-commands", ("m1-single-repo-up",))

        frontier = sprout_frontier.load_frontier(self.root)

        self.assertEqual(frontier["implemented"], ["m1-single-repo-up"])
        self.assertEqual([row["id"] for row in frontier["ready"]], ["m1-core-commands"])
        self.assertEqual(frontier["missingDependencies"], [])


if __name__ == "__main__":
    unittest.main()
