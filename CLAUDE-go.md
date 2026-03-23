# Go conventions

## Implementation approach

When implementing from a spec, follow the spec closely but flag concerns
rather than silently working around them. If an interface definition seems
inconsistent with the rest of the codebase or a design decision looks wrong,
note it before proceeding rather than inventing a fix unilaterally.

Prefer configurable values over hardcoded constants. Config fields belong in
the config struct and must have a corresponding entry in the config file
documentation.

Update the relevant README and CLAUDE.md after making code changes that affect
interfaces, commands, flags, or behavior.

Write table-driven tests for pure functions in pkg/. For CLI commands, test
the core logic (buildRows, plan.Build, classify) rather than the Cobra wiring.
