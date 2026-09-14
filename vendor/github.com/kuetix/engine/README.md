# Kuetix Engine

Kuetix Engine is the core runtime and modular workflow engine that powers Kuetix applications. It provides a lightweight bootstrap, dependency injection, configuration, and a workflow subsystem to orchestrate tasks and services.

## Features
- Modular architecture with pluggable modules under `modules/`
- Workflow runtime with support for `workflow`, `feature`, and `solution` types
- Hierarchical execution: solutions orchestrate features; features orchestrate workflows
- Dependency injection helpers in `boot/` and `modules/`
- Structured logging under `pkg/helpers/logger/`
- Config profiles for development and production in `runtime/etc/`
- Example workflows in `runtime/workflows/`
- **SimplifiedWSL** (`.swsl`) — concise workflow syntax with `->` chaining and `<-` error binding
- **Nested const blocks** — objects and arrays in `const` blocks with automatic type conversion
- **On Success When** — conditional transitions based on step result expressions

## Getting Started

### Prerequisites
- Go 1.21+
- Make (optional, for convenience)
- Docker (optional, for running examples via docker-compose)

### Clone
```bash
git clone https://github.com/kuetix/engine.git
cd engine
```

### Build
Using Makefile:
```bash
make kue
```

Or directly with Go:
```bash
go build ./...
```

### Quick Start
```bash
# Build the kue CLI
make kue

# Create a new application project
./runtime/bin/kue create --name my_project --app-type cli

# Add a module to an existing project
./runtime/bin/kue add module my_module

# Update the module cache (di.go, meta.go, modules.json)
./runtime/bin/kue update

# Show project components
./runtime/bin/kue show

# Show help
./runtime/bin/kue --help
```

See [Kue CLI Documentation](#kue-cli) below for detailed usage.

### Configuration
Configuration files live under `runtime/etc/` with separate profiles, e.g.:
- `runtime/etc/development`
- `runtime/etc/production`

Logs are written to `runtime/log/` by default.

### Workflows
Sample workflows can be found in:
- `runtime/workflows/common`
- `runtime/workflows/workflow`
- `runtime/workflows/wsl_hello_world`
- `runtime/workflows/hierarchical_example`

#### SimplifiedWSL
Kuetix Engine supports **SimplifiedWSL**, a streamlined syntax for defining workflows without verbose boilerplate. See [SimplifiedWSL Documentation](docs/SIMPLIFIED_WSL.md) for details.

#### Hierarchical Execution
Workflows can be organized hierarchically: solutions orchestrate features, features orchestrate workflows, all sharing a single `WorkerSessionContext`. See [Hierarchical Execution Documentation](docs/HIERARCHICAL_EXECUTION.md) for details.

## Project Layout
```
boot/          # Bootstrapping helpers and services
cmd/           # CLI entrypoints
  kue/         # Main workflow CLI tool (create, add, update, show, templates, version)
docs/          # Additional documentation
event/         # Event types and handlers
internal/      # Internal WSL parser (wsl, simplified_wsl)
modules/       # Optional modules and integrations
packaging/     # Distribution packages (Homebrew, Debian)
pkg/           # Public packages (workflow, domain, services, helpers)
runtime/       # Config, logs, and example workflows
tests/         # Test suites and fixtures
```

## CLI Tools

### Kue CLI

The `kue` command is the primary CLI tool for Kuetix workflow development. It provides project scaffolding, code generation, module cache management, and more.

**Installation:**
```bash
# Build locally
make kue

# Install globally
go install github.com/kuetix/engine/cmd/kue@latest

# Install via Homebrew (macOS/Linux)
brew install kuetix/tap/kue

# Install via APT (Ubuntu/Debian)
sudo apt install ./dist/kue_0.1.0_amd64.deb
```

**Commands:**

#### `kue create` - Create a new project
Create a new application or package from a template:
```bash
kue create --name my_app --app-type cli
kue create -n my_api -a api
kue create -n my_package -a package -o ./projects
```

Options:
- `--name, -n`: Project name (required)
- `--app-type, -a`: Application type (default: cli)
  - `cli`: CLI application
  - `api`: Web API application
  - `consumer`: AMQP queue consumer (Apache ActiveMQ)
  - `service`: Background service/daemon
  - `package`: Reusable package with workflows/features/solutions
  - `all`: Full application with all types
- `--output, -o`: Output directory (default: current directory)
- `--force, -f`: Force creation without confirmation

#### `kue add` - Add components to an existing project
Add modules, workflows, features, solutions, or transitions:
```bash
kue add module payment
kue add workflow order-processing
kue add feature payment-gateway
kue add solution e-commerce
kue add transition payment ProcessPayment
```

Subcommands: `module`, `workflow`, `feature`, `solution`, `transition`

#### `kue update` - Update module cache
Regenerate the module cache files (`di.go`, `meta.go`, `modules.json`) after adding or modifying modules:
```bash
kue update
kue update --verbose
kue update --quiet
```

Options:
- `--verbose, -v`: Verbose mode
- `--quiet, -q`: Quiet mode

#### `kue show` - Show project components
List applications, solutions, features, and workflows in the current project:
```bash
kue show
```

#### `kue templates` - Manage template cache
Download, update, or inspect the local template cache:
```bash
kue templates download
kue templates update
kue templates clear
kue templates status
```

#### `kue register` - Register a new account
Register a Kuetix account:
```bash
kue register --email user@example.com --password my-secret
kue register --email user@example.com --password my-secret --name "John Doe" --username johndoe
```

Options:
- `--email`: Email (required)
- `--password`: Password (required)
- `--name`: Display name (optional)
- `--username`: Username (optional)
- `--host, -h`: API host (default: api.kuetix.com)

#### `kue profile` - Manage your profile
Get and update authenticated profile data:
```bash
kue profile get
kue profile update --name "John Doe"
kue profile update --username johndoe
kue profile update --name "John Doe" --username johndoe
```

Subcommands:
- `get`: Get current profile (requires login)
- `update`: Update `name` and/or `username` (requires login)

#### `kue version` - Show version information
```bash
kue version
```

**Global Template Options:**

Templates are loaded from an external source. The priority is: local path > git > web URL.

```bash
# Use a specific template version
kue --template-version 0.1.0 create --name myapp

# Use templates from a local directory
kue --template-path /path/to/templates create --name myapp

# Use templates from a git repository
kue --template-git https://github.com/user/templates.git create --name myapp

# Use a custom URL
kue --template-url https://example.com/templates/ create --name myapp
```

Environment variables: `KUE_TEMPLATE_URL`, `KUE_TEMPLATE_VERSION`, `KUE_TEMPLATE_PATH`, `KUE_TEMPLATE_GIT`

See [Template System Documentation](docs/TEMPLATE_SYSTEM.md) for full details.

## Workflow Language (WSL)

### Regular WSL

Workflows are defined in `.wsl` files using the Workflow Specification Language:

```wsl
module example

const {
    event: "greet",
    version: "1.0.0",
    cfg: {
        timeout: 30000,
        headers: [
            { key: "Content-Type", value: "application/json" }
        ]
    }
}

workflow main {
    start: Execute

    state Execute {
        action myModule.DoSomething(
            timeout: $constants.cfg.timeout
        ) as result
        on success when <<result.ok>> == true -> Done
        on success -> Retry
    }

    state Done {
        end ok
    }
}
```

**Const blocks** support nested objects and arrays with automatic type conversion (string, int64, float64, bool, null).

**`on success when`** allows conditional transitions based on the step result.

### SimplifiedWSL

SimplifiedWSL (`.swsl`) provides a concise syntax without workflow wrappers:

```swsl
module example

feature  # Declare workflow type (feature, solution, workflow, or custom)

const {
    msg: "Hello"
}

def errors.OnAnyError() as errorHandler -> .

speak.Say(on: "message", v: $constants.msg) <- errorHandler -> common.Response(message: "Done") -> .
```

See [SimplifiedWSL Documentation](docs/SIMPLIFIED_WSL.md) for full syntax reference.

### Hierarchical Execution

Workflows can be organized into three levels that share a single `WorkerSessionContext`:

```
Solution  →  can call features, workflows, and actions
  Feature →  can call workflows and actions
    Workflow → can call actions
```

**Regular WSL:**
```wsl
feature payment_gateway {
    start: Step1

    state Step1 {
        action workflow validate_payment   # loads validate_payment.wsl
        on success -> Step2
    }

    state Step2 {
        action workflow process_payment    # loads process_payment.wsl
        on success -> Done
    }

    state Done {
        end ok
    }
}
```

**SimplifiedWSL:**
```swsl
feature payment_gateway

workflow:validate_payment() <-
workflow:process_payment() -> .
```

See [Hierarchical Execution Documentation](docs/HIERARCHICAL_EXECUTION.md) for full details.

## Docker Support

Run Kuetix Engine workflows using Docker Compose:

```bash
# Start all services (including workflow runner)
docker-compose up

# Start specific service
docker-compose up workflow

# Run a workflow in a container
docker-compose run --rm workflow /bin/bash -c "cd /app && kue update && kue show"
```

The `docker-compose.yaml` includes a dedicated `workflow` service for running workflows in containers.

## Packaging

### Homebrew (macOS/Linux)

The Homebrew formula is available at `packaging/homebrew/kue.rb`. To create a tap:

```bash
# Create a Homebrew tap repository
brew tap kuetix/tap https://github.com/kuetix/homebrew-tap

# Install kue
brew install kue
```

### Debian/Ubuntu Package

Build a `.deb` package for Ubuntu/Debian:

```bash
# Build the package
cd packaging/debian
./build-deb.sh

# Install the package
sudo dpkg -i ../../dist/kue_0.1.0_amd64.deb
```

The build script creates a Debian package in the `dist/` directory.

## Development
- Use `make test` to run tests.
- Module definition is in `go.mod`.
- See [CLI Documentation](docs/CLI.md) for the full `kue` command reference.

## License
This project is licensed under the terms of the LICENSE file included in this repository. See `LICENSE` and `NOTICE` for details.

## Trademark
See `TRADEMARK.md` for the trademark policy.

---

Kuetix™ is an unregistered trademark of Anar Alishov. All rights reserved.
The Kuetix™ name and logo are not covered by this license.
