from __future__ import annotations

import re
import unittest
from pathlib import Path
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[2]
GOVERNANCE_DOCUMENTS = (
    "README.md",
    "CONTRIBUTING.md",
    "GOVERNANCE.md",
    "MAINTAINERS.md",
    "ROADMAP.md",
    "RELEASES.md",
    "COMPATIBILITY.md",
    "TELEMETRY.md",
    "TRADEMARKS.md",
    "PROJECT_HEALTH.md",
    "SECURITY.md",
    "CODE_OF_CONDUCT.md",
    "docs/decisions/README.md",
)
LOCAL_LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


class GovernanceDocumentTests(unittest.TestCase):
    def test_readme_indexes_governance_contract(self) -> None:
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        linked = {
            urlparse(target.strip().split(maxsplit=1)[0]).path
            for target in LOCAL_LINK.findall(readme)
        }
        for document in GOVERNANCE_DOCUMENTS:
            with self.subTest(document=document):
                self.assertTrue((ROOT / document).is_file())
                if document != "README.md":
                    self.assertIn(document, linked)

    def test_contributor_flow_names_checks_reviews_and_dco(self) -> None:
        contributing = (ROOT / "CONTRIBUTING.md").read_text(encoding="utf-8")
        for expected in (
            "make check",
            "git commit -s",
            "Developer Certificate of Origin 1.1",
            "not a Contributor License Agreement",
            "GOVERNANCE.md",
            "SECURITY.md",
            "docs/decisions/README.md",
        ):
            with self.subTest(expected=expected):
                self.assertIn(expected, contributing)

        makefile = (ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertRegex(makefile, r"(?m)^check:")
        self.assertRegex(makefile, r"(?m)^automation-check:")

    def test_governance_exposes_required_paths_and_open_gaps(self) -> None:
        governance = (ROOT / "GOVERNANCE.md").read_text(encoding="utf-8")
        for expected in (
            "MAINTAINERS.md",
            "docs/decisions/README.md",
            "CONTRIBUTING.md",
            "RELEASES.md",
            "COMPATIBILITY.md",
            "SECURITY.md",
            "multiple organizations",
        ):
            with self.subTest(expected=expected):
                self.assertIn(expected, governance)

        maintainers = (ROOT / "MAINTAINERS.md").read_text(encoding="utf-8")
        self.assertIn("Bus factor is one", maintainers)
        self.assertIn("not operational", maintainers)


if __name__ == "__main__":
    unittest.main()
