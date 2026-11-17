from __future__ import annotations

import json
import re
import unittest
from pathlib import Path
from urllib.parse import unquote, urlparse

ROOT = Path(__file__).resolve().parents[2]
FEATURES = ROOT / "docs" / "features"
TASKS = ROOT / ".sprout" / "tasks"
METADATA = re.compile(r"\A<!-- sprout-task\n(.*?)\n-->", re.DOTALL)
LOCAL_LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


class FeatureDocumentationTests(unittest.TestCase):
    def test_completed_documents_are_reader_facing(self) -> None:
        documents = sorted(FEATURES.glob("m*-*.md"))
        self.assertTrue(documents)
        for document in documents:
            with self.subTest(document=document.name):
                text = document.read_text(encoding="utf-8")
                metadata = METADATA.match(text)
                self.assertIsNotNone(metadata)
                assert metadata is not None
                provenance = json.loads(metadata.group(1))
                self.assertEqual(provenance["id"], document.stem)
                self.assertEqual(provenance["status"], "implemented")
                for forbidden in (
                    "Execute this task end to end.",
                    "## Execution checklist",
                    "## Original request",
                    "Do not commit, push, or mutate shared infrastructure",
                ):
                    self.assertNotIn(forbidden, text)
                self.assertNotRegex(text, r"(?m)^- \[ \]")
                for heading in (
                    "Scope and dependencies", "Behavior", "Constraints",
                    "Verified acceptance criteria", "Implementation details",
                    "Verification", "Validation evidence",
                ):
                    self.assertRegex(text, rf"(?m)^## {re.escape(heading)}\n\n\S")
                self.assertFalse((TASKS / document.name).exists())

    def test_feature_index_covers_every_document_exactly_once(self) -> None:
        index = (ROOT / "docs" / "features.md").read_text(encoding="utf-8")
        linked = [target for target in LOCAL_LINK.findall(index) if target.startswith("features/")]
        expected = [f"features/{path.name}" for path in FEATURES.glob("m*-*.md")]
        self.assertCountEqual(linked, expected)
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("docs/features.md", LOCAL_LINK.findall(readme))

    def test_feature_document_links_resolve(self) -> None:
        documents = [ROOT / "docs" / "features.md", *FEATURES.glob("m*-*.md")]
        for document in documents:
            for target in LOCAL_LINK.findall(document.read_text(encoding="utf-8")):
                parsed = urlparse(target)
                if parsed.scheme or parsed.netloc or not parsed.path:
                    continue
                resolved = (document.parent / unquote(parsed.path)).resolve()
                with self.subTest(document=document.name, target=target):
                    self.assertTrue(resolved.is_relative_to(ROOT.resolve()))
                    self.assertTrue(resolved.exists())


if __name__ == "__main__":
    unittest.main()
