# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] – 2025-11-22

### Added
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Roadmap and initial documentation for upcoming features.
- CI workflow skeleton for builds and tests.
- Example usage snippets for README.

## [0.1.0] \u2013 2025-11-22

### Added
- Initial public release of `uuid`.
- `UUID` type: zero-value safe and lightweight wrapper around UUIDs.
- Constructors and helpers: `New`, `NewV4`, `Parse`, `MustParse`, `IsZero`.
- Stringer support: `String()` and `FromString` helpers.
- Encoding support: `MarshalJSON` / `UnmarshalJSON` and `MarshalText` / `UnmarshalText`.
- Database support: implementations of `database/sql.Scanner` and `driver.Valuer` for seamless DB usage.
- Validation helpers and clear error types for parse/validation failures.
- Unit tests and basic benchmarks covering core functionality.
- `go.mod` and minimal dependency surface for easy reuse.
- Continuous Integration: GitHub Actions for tests on multiple Go versions.
- Example usage and minimal README snippets demonstrating generation, parsing, and JSON marshal/unmarshal.
- `LICENSE` file (choose an OSI\-approved license, e.g. `MIT` or `Apache-2.0`).

### Changed
- N/A

### Fixed
- N/A

### Security
- N/A

---

For details on how to contribute, see `CONTRIBUTING.md`. For licensing, add the chosen license to `LICENSE`.
