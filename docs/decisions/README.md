# Architecture Decision Records

This directory contains durable decisions that constrain Agent Box architecture, compatibility, security, persistence, or lifecycle behavior.

## Naming and numbering

Use `NNNN-short-kebab-title.md`. Allocate the next unused four-digit number; never renumber an accepted record. The number is an identifier, not priority or milestone order.

## Required sections

Each ADR must contain:

1. title and metadata (`Status`, `Date`, and `Scope`);
2. context and the invariant or pressure requiring a decision;
3. decision;
4. alternatives considered;
5. consequences, including failure, security, portability, and operational costs where relevant;
6. concrete evidence or upstream contracts; and
7. measurable revisit triggers.

Valid statuses are `Proposed`, `Accepted`, `Superseded by ADR NNNN`, and `Rejected`. Change a proposed ADR in place. Once accepted, preserve its historical decision; use a new ADR to supersede it when the contract changes. Corrections that do not alter the decision may be made in place with clear commit history.

Do not use an ADR to manufacture evidence. Separate verified behavior, design intent, and unresolved platform gates explicitly.

## Index

| ADR | Status | Decision |
| --- | --- | --- |
| [0001](0001-milestone-0-substrate-gate.md) | Accepted for the Linux implementation lane | Gate product implementation on measured substrate evidence |
