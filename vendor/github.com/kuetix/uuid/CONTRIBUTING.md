# Contributing to kuetix/uuid

Thank you for considering a contribution! This project follows a lightweight workflow to keep things simple and friendly.

## Getting started
- Go 1.21+ is recommended. The module should work with the version declared in `go.mod`.
- Clone the repo and run tests:
  ```bash
  go test ./...
  ```

## Development workflow
1. Fork the repository and create your branch from `main`.
2. Write clear, minimal changes focused on a single topic.
3. Add or update tests where applicable: place tests under `test/`.
4. Ensure `go fmt`/`go vet` look good and tests pass locally.
5. Open a pull request with a succinct title and description of the change and motivation.

## Commit messages
- Use conventional, descriptive messages (e.g., `fix:`, `feat:`, `docs:`).
- Keep the first line short; add context in the body when needed.

## Code style
- Match the existing code style.
- Keep public API surface minimal and well-documented.

## Reporting issues
- Search existing issues first.
- Include Go version, OS, steps to reproduce, expected/actual behavior.

## Security
Please do NOT open public issues for security reports. See `SECURITY.md` for the private reporting process.

## License
By contributing, you agree that your contributions are licensed under the MIT License of this repository.
