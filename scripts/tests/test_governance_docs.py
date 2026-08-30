from __future__ import annotations

import re
import unittest
from pathlib import Path
from urllib.parse import unquote, urlparse

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
    def test_local_links_resolve_inside_repository(self) -> None:
        failures: list[str] = []
        for relative_name in GOVERNANCE_DOCUMENTS:
            document = ROOT / relative_name
            self.assertTrue(document.is_file(), relative_name)
            for raw_target in LOCAL_LINK.findall(document.read_text(encoding="utf-8")):
                target = raw_target.strip().split(maxsplit=1)[0]
                parsed = urlparse(target)
                if parsed.scheme or parsed.netloc or target.startswith("#"):
                    continue
                path_text = unquote(parsed.path)
                if not path_text:
                    continue
                resolved = (document.parent / path_text).resolve()
                try:
                    resolved.relative_to(ROOT.resolve())
                except ValueError:
                    failures.append(f"{relative_name}: link escapes repository: {target}")
                    continue
                if not resolved.exists():
                    failures.append(f"{relative_name}: missing link target: {target}")
        self.assertEqual(failures, [])

    def test_readme_indexes_governance_contract(self) -> None:
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        required = {
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
        }
        linked = {
            urlparse(target.strip().split(maxsplit=1)[0]).path
            for target in LOCAL_LINK.findall(readme)
        }
        self.assertEqual(required - linked, set())

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
