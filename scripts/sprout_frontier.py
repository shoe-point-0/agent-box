#!/usr/bin/env python3
"""Report dependency-ready, blocked, and intentionally deferred Sprout tasks."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

TASK_ID = re.compile(r"\bm\d-[a-z0-9-]+\b")
DEPENDENCIES = re.compile(r"Dependencies:\s*([^\n.]+)")
TITLE = re.compile(r"^#\s+(.+)$", re.MULTILINE)
METADATA = re.compile(r"\A<!-- sprout-task\n(.*?)\n-->", re.DOTALL)


def feature_task_id(path: Path) -> str:
    text = path.read_text(encoding="utf-8")
    match = METADATA.match(text)
    if match:
        provenance = json.loads(match.group(1))
        task_id = provenance.get("id")
        if isinstance(task_id, str) and task_id:
            return task_id
    return path.stem


@dataclass(frozen=True)
class Task:
    id: str
    title: str
    dependencies: tuple[str, ...]
    explicitly_deferred: bool


def parse_task(path: Path) -> Task:
    text = path.read_text(encoding="utf-8")
    title_match = TITLE.search(text)
    dependency_match = DEPENDENCIES.search(text)
    dependencies = tuple(TASK_ID.findall(dependency_match.group(1))) if dependency_match else ()
    explicitly_deferred = (
        path.stem.startswith("m6-")
        or path.stem == "m5-darwin-arm64-hardware-gate"
        or "Final Darwin hardware evidence input" in text
    )
    return Task(
        id=path.stem,
        title=title_match.group(1) if title_match else path.stem,
        dependencies=dependencies,
        explicitly_deferred=explicitly_deferred,
    )


def load_frontier(root: Path) -> dict[str, Any]:
    tasks_dir = root / ".sprout" / "tasks"
    features_dir = root / "docs" / "features"
    tasks = {path.stem: parse_task(path) for path in sorted(tasks_dir.glob("*.md"))}
    implemented = {feature_task_id(path) for path in features_dir.glob("*.md")}
    known = set(tasks) | implemented

    missing_dependencies = sorted(
        {dependency for task in tasks.values() for dependency in task.dependencies if dependency not in known}
    )
    cycles = dependency_cycles(tasks, implemented)

    deferred = {task.id for task in tasks.values() if task.explicitly_deferred}
    changed = True
    while changed:
        changed = False
        for task in tasks.values():
            if task.id in deferred:
                continue
            unmet = [dependency for dependency in task.dependencies if dependency not in implemented]
            if unmet and all(dependency in deferred for dependency in unmet):
                deferred.add(task.id)
                changed = True

    ready: list[Task] = []
    blocked: list[dict[str, object]] = []
    deferred_rows: list[dict[str, object]] = []
    for task in sorted(tasks.values(), key=lambda item: item.id):
        unmet = [dependency for dependency in task.dependencies if dependency not in implemented]
        row = {**asdict(task), "unmet_dependencies": unmet}
        if task.id in deferred:
            deferred_rows.append(row)
        elif not unmet:
            ready.append(task)
        else:
            blocked.append(row)

    return {
        "schemaVersion": "abox.sprout.frontier/v1",
        "ready": [asdict(task) for task in ready],
        "blocked": blocked,
        "deferred": deferred_rows,
        "implemented": sorted(implemented),
        "missingDependencies": missing_dependencies,
        "cycles": cycles,
    }


def dependency_cycles(tasks: dict[str, Task], implemented: set[str]) -> list[list[str]]:
    visiting: set[str] = set()
    visited: set[str] = set()
    stack: list[str] = []
    cycles: list[list[str]] = []

    def visit(task_id: str) -> None:
        if task_id in visited or task_id in implemented or task_id not in tasks:
            return
        if task_id in visiting:
            start = stack.index(task_id)
            cycle = stack[start:] + [task_id]
            if cycle not in cycles:
                cycles.append(cycle)
            return
        visiting.add(task_id)
        stack.append(task_id)
        for dependency in tasks[task_id].dependencies:
            visit(dependency)
        stack.pop()
        visiting.remove(task_id)
        visited.add(task_id)

    for task_id in sorted(tasks):
        visit(task_id)
    return cycles


def print_human(frontier: dict[str, Any]) -> None:
    for category in ("ready", "blocked", "deferred"):
        rows = frontier[category]
        print(f"{category.upper()} ({len(rows)})")
        for row in rows:
            suffix = ""
            if row.get("unmet_dependencies"):
                suffix = " <- " + ", ".join(row["unmet_dependencies"])
            print(f"  {row['id']}: {row['title']}{suffix}")
    print(f"IMPLEMENTED ({len(frontier['implemented'])})")
    if frontier["missingDependencies"]:
        print("MISSING DEPENDENCIES: " + ", ".join(frontier["missingDependencies"]))
    if frontier["cycles"]:
        print("CYCLES: " + json.dumps(frontier["cycles"]))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--next", action="store_true")
    parser.add_argument("--assert-terminal", action="store_true")
    args = parser.parse_args()

    root = args.root.resolve()
    frontier = load_frontier(root)
    if frontier["missingDependencies"] or frontier["cycles"]:
        if args.json:
            print(json.dumps(frontier, indent=2))
        else:
            print_human(frontier)
        return 2

    if args.next:
        ready = frontier["ready"]
        if ready:
            print(ready[0]["id"])
            return 0
        return 1
    if args.assert_terminal:
        if frontier["ready"] or frontier["blocked"]:
            print_human(frontier)
            return 1
        print("Sprout Linux frontier exhausted; only deferred or deferred-blocked tasks remain.")
        return 0
    if args.json:
        print(json.dumps(frontier, indent=2))
    else:
        print_human(frontier)
    return 0


if __name__ == "__main__":
    sys.exit(main())
