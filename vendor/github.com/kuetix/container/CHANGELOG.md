# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] – 2025-11-21

### Added

Provides a simple thread-safe dependency injection container with singleton and factory registries, parameter storage, and introspection helpers.

Features include:
- Initial release of the github.com/kuetix/container package.
- Thread-safe singleton registry (ToFetch, ToFetchFunc, Fetch).
- Thread-safe factory registry with on-demand resolution (ToResolve, Resolve).
- Unified lookup via Get, resolving either singleton or factory.
- Parameters store (ToParameter, Parameter) for lightweight config values.
- Introspection helpers:
- Has, CanFetch, CanResolve, HasParameter.
- Reset() helper for testing.
- Project documentation: README.md, LICENSE, TRADEMARK.md.
