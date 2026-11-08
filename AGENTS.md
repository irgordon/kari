# AGENTS.md

# Karı Development Instructions

This repository contains Karı, a security-focused server control panel composed of:

- React/Vite UI
- Go API daemon
- Rust privileged system agent
- PostgreSQL
- gRPC over a local Unix socket
- Nginx and systemd integration

Read `docs/TECHNICAL_SPEC.md` before making architectural or security-sensitive changes.

The technical specification defines WHAT the system must do.

This file defines HOW changes should be implemented.

---

## 1. Primary Engineering Rule

Prefer the simplest correct implementation.

Code should be:

1. Correct
2. Secure
3. Easy to read
4. Easy to delete or replace
5. Easy to test
6. As small as practical

Do not optimize for theoretical future requirements.

Do not introduce abstractions because they might become useful later.

Do not increase architectural complexity unless the current requirement demands it.

---

## 2. Keep Changes Small

Make the smallest change that completely solves the requested problem.

Do not:

- Refactor unrelated code.
- Rename unrelated symbols.
- Reformat unrelated files.
- Move files without a concrete reason.
- Introduce a framework to solve a local problem.
- Add extension points without a current consumer.
- Generalize code for hypothetical future use cases.
- Replace working code merely with code that is stylistically preferred.

If five straightforward lines solve a problem cleanly, do not replace them with a new abstraction.

Prefer local reasoning over distributed behavior.

---

## 3. Avoid Overengineering

Do not introduce:

- unnecessary interfaces
- unnecessary traits
- unnecessary factories
- unnecessary dependency injection
- unnecessary generic types
- unnecessary wrapper types
- unnecessary configuration
- unnecessary middleware
- unnecessary helper packages
- speculative caching
- speculative concurrency
- speculative extensibility
- speculative retry systems

An interface or trait should exist when there is an actual abstraction boundary, security boundary, or multiple implementations that justify it.

Do not create an interface solely to make a function mockable.

Do not implement functionality that was not requested.

YAGNI applies.

---

## 4. Avoid Spaghetti Code

Keep execution paths obvious.

Prefer:

- early returns
- explicit error handling
- small focused functions
- shallow nesting
- clear data flow
- obvious ownership
- direct function calls
- dependency direction that can be understood from the directory structure

Avoid:

- deeply nested conditionals
- hidden control flow
- global mutable state
- circular dependencies
- callbacks when normal control flow works
- chains of tiny forwarding functions
- excessive middleware layers
- helpers that mutate distant state
- abstractions whose behavior requires reading many files to understand

A developer should be able to determine the major execution path by reading code from top to bottom.

---

## 5. Abstraction Rule

Use the lowest level of abstraction that remains clean.

Before creating a new abstraction, ask:

1. Is there more than one real implementation?
2. Does this isolate a meaningful security or system boundary?
3. Does it substantially simplify the caller?
4. Does existing code already provide the abstraction?

If the answer to all four is no, do not add the abstraction.

Do not create abstractions around abstractions.

Prefer:

    handler -> service -> concrete operation

over:

    handler -> interface -> coordinator -> manager
            -> provider -> adapter -> operation

unless those additional boundaries have concrete responsibilities.

---

## 6. Testing Philosophy

Tests should protect real behavior, security boundaries, and regressions.

Keep speculative testing minimal.

Write tests for:

- requested behavior
- bugs being fixed
- security-sensitive validation
- authorization decisions
- privilege boundaries
- destructive filesystem operations
- parsing or validation with meaningful edge cases
- code where failure could damage system state
- established behavior that could realistically regress

Do not write tests solely to increase coverage.

Do not test trivial language behavior.

Do not create large numbers of hypothetical edge-case tests without evidence they matter.

Do not duplicate the same behavior across multiple test layers unless the layers protect different risks.

Prefer a small number of high-value tests.

---

## 7. Test Scope

Start with the narrowest relevant test.

For a localized change:

1. Run the directly affected tests.
2. Run the affected package or crate tests if appropriate.
3. Run broader tests only when the change crosses boundaries or the narrower tests reveal a reason.

Do not automatically run expensive integration or end-to-end suites for isolated changes unless repository requirements demand them.

Do not create new integration infrastructure for a small unit-level change.

---

## 8. Security Is Not Speculative Complexity

Do not interpret the simplicity rules as permission to weaken security.

Security boundaries described in `docs/TECHNICAL_SPEC.md` are requirements.

In particular:

- Never construct shell command strings from user input.
- Use discrete process arguments.
- Validate externally controlled input before privileged operations.
- Preserve privilege separation between the Go API and Rust agent.
- Keep filesystem operations scoped to approved paths.
- Treat authentication and authorization as explicit boundaries.
- Do not expose secrets in logs, errors, telemetry, or UI responses.
- Preserve protections around destructive operations.
- Fail closed when authorization or identity cannot be established.

Security checks may be redundant when they protect different trust boundaries.

Do not remove a security check merely to reduce line count.

---

## 9. Rust Agent Rules

The Rust agent runs with elevated privileges.

Treat every externally supplied value as untrusted.

Prefer:

- standard library functionality
- explicit types
- `Result`
- bounded resources
- deterministic filesystem operations
- direct `std::process::Command` argument construction

Avoid:

- `Command::new("sh")`
- `bash -c`
- interpolated shell commands
- unnecessary `unsafe`
- unnecessary cloning
- unbounded channels
- detached background work without ownership
- panics for recoverable failures

Use `unsafe` only when required and document the invariant being relied upon.

Privileged code should be boring.

---

## 10. Go API Rules

Keep HTTP concerns separate from business behavior without creating unnecessary layers.

Handlers should primarily:

1. Parse input.
2. Validate input.
3. Authorize the operation.
4. Call the relevant service or operation.
5. Map the result to an HTTP response.

Business behavior should not depend directly on HTTP request or response objects.

Prefer explicit dependencies.

Avoid service-locator patterns and global mutable application state.

Do not introduce an interface for every service automatically.

---

## 11. React UI Rules

Keep components focused on presentation and interaction.

Prefer:

- simple components
- explicit state
- existing React primitives
- Go API handling for security-sensitive cookie operations

Avoid:

- unnecessary client-side state libraries
- generic component frameworks built inside the repository
- premature component abstraction
- duplicating server authorization logic in the browser

Client-side checks improve UX.

They are not authorization boundaries.

---

## 12. Database Rules

Prefer explicit SQL and straightforward relational modeling.

Do not:

- fetch unbounded result sets
- generate migrations for unrelated changes
- hide simple queries behind unnecessary repository abstractions
- use JSONB where normal relational columns provide a clearer model
- add indexes without a concrete query or constraint requiring them

Database migrations must be deterministic and narrowly scoped.

Never silently destroy existing data.

---

## 13. Dependencies

Prefer the standard library and existing dependencies.

Before adding a dependency, verify that:

1. Existing code does not already solve the problem.
2. The standard library cannot reasonably solve it.
3. The dependency removes meaningful complexity.
4. The dependency is justified by the requested change.

Do not add a package for trivial functionality.

Do not replace existing dependencies without a task-specific reason.

---

## 14. Error Handling

Errors should provide enough context to diagnose the failure without exposing sensitive information.

Prefer explicit error propagation.

Do not:

- silently swallow errors
- log secrets
- expose credentials
- expose internal filesystem paths unnecessarily
- convert every error into a generic success/failure boolean
- create elaborate custom error hierarchies without a concrete need

Add context at meaningful system boundaries.

---

## 15. Comments

Comments should explain WHY, not narrate WHAT the code visibly does.

Good comments explain:

- security assumptions
- non-obvious invariants
- compatibility constraints
- unusual operating-system behavior
- why an apparently simpler implementation is unsafe

Avoid comments such as:

    // Increment counter
    counter++

Do not write comments simply to make code appear documented.

---

## 16. Refactoring

Refactor when necessary to implement the requested change safely and cleanly.

Do not perform opportunistic repository-wide cleanup.

If existing code is imperfect but unrelated to the task, leave it alone unless it prevents a correct implementation.

A refactor should reduce complexity, not redistribute it.

---

## 17. Performance

Do not optimize without a reason.

However, privileged daemons and long-running services must avoid obviously unbounded resource consumption.

Use bounded operations where external input can influence:

- memory allocation
- goroutine/task creation
- channel growth
- result-set size
- filesystem traversal
- log accumulation

Do not introduce caching, pooling, batching, concurrency, or lock-free structures without a demonstrated requirement.

Simple synchronous code is preferred when it meets the requirement.

---

## 18. Before Writing Code

Before modifying the repository:

1. Read the relevant existing implementation.
2. Identify the smallest affected area.
3. Check for an existing pattern that already solves the problem.
4. Identify relevant security boundaries.
5. Identify the minimum tests needed.
6. Implement the smallest complete change.

Do not begin by creating new architecture.

---

## 19. Before Finishing

Before considering a task complete:

1. Inspect the diff.
2. Remove unrelated changes.
3. Remove unused abstractions.
4. Remove speculative code.
5. Remove unnecessary comments.
6. Confirm error paths are handled.
7. Confirm trust boundaries remain enforced.
8. Run the narrowest relevant tests.
9. Run broader validation only when appropriate.
10. Confirm the implementation matches the requested behavior.

Ask:

> Could this change be simpler without sacrificing correctness, security, or readability?

If yes, simplify it.

---

## 20. Definition of Done

A change is done when:

- the requested behavior works
- relevant security boundaries remain intact
- relevant tests pass
- the implementation is understandable
- there are no unrelated modifications
- there are no speculative features
- there are no unnecessary abstractions

Stop there.

Do not continue improving unrelated code.
