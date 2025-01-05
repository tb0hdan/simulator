# List of project decisions

- Directories were restructured in accordance with the [Go Project Layout](https://github.com/golang-standards/project-layout/) guidelines.
- Server structure was modeled after the `net/http` package given that reference itself is a simplified HTTP server.
- Introduced `testify/suite` to preserve server state between tests and provide a more organized test suite.
- Replaced `require` with `assert` since require breaks test flow and is prone to race conditions.
