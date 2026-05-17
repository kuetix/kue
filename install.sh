#!/bin/bash

# Kuetix Engine Installation Script
# Installs built binaries following go install path resolution: GOBIN > GOPATH/bin > HOME/go/bin > /usr/local/bin

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Installation directory: follow go install path resolution
# GOBIN > GOPATH/bin > HOME/go/bin > /usr/local/bin (fallback)
if [ -z "$INSTALL_DIR" ]; then
    if [ -n "$GOBIN" ]; then
        INSTALL_DIR="$GOBIN"
    elif [ -n "$GOPATH" ]; then
        INSTALL_DIR="$GOPATH/bin"
    elif [ -n "$HOME" ]; then
        INSTALL_DIR="$HOME/go/bin"
    else
        INSTALL_DIR="/usr/local/bin"
    fi
fi
BUILD_DIR="runtime/bin"

# Available binaries
BINARIES=("kue")

# Display help message
show_help() {
    cat << EOF
${BLUE}Kuetix Engine Installation Script${NC}

${YELLOW}USAGE:${NC}
    ./install.sh [OPTIONS] [BINARIES...]

${YELLOW}DESCRIPTION:${NC}
    Install kue binary.
    Installation directory is resolved in the following order:
      1. --dir / -d option
      2. \$INSTALL_DIR environment variable
      3. \$GOBIN environment variable         (if set)
      4. \$GOPATH/bin                         (if \$GOPATH is set)
      5. \$HOME/go/bin                        (Go default)
      6. /usr/local/bin                      (last resort fallback)

${YELLOW}OPTIONS:${NC}
    -h, --help          Show this help message
    -d, --dir DIR       Installation directory
    -l, --list          List available binaries

${YELLOW}AVAILABLE BINARIES:${NC}
    kue                 Workflow CLI tool

${YELLOW}EXAMPLES:${NC}
    # Install kue binary
    ./install.sh
    ./install.sh kue

    # Install to custom directory
    ./install.sh --dir ~/bin kue

    # List available binaries
    ./install.sh --list

${YELLOW}NOTES:${NC}
    - Installation follows go install path resolution: GOBIN > GOPATH/bin > HOME/go/bin > /usr/local/bin
    - Binary must be built before installation (run 'make kue' first)
    - Custom installation directory must be in your PATH

EOF
}

# List available binaries
list_binaries() {
    echo -e "${BLUE}Available binaries:${NC}"
    for binary in "${BINARIES[@]}"; do
        if [ -f "$BUILD_DIR/$binary" ]; then
            echo -e "  ${GREEN}✓${NC} $binary (built)"
        else
            echo -e "  ${RED}✗${NC} $binary (not built)"
        fi
    done
}

# Install a single binary
install_binary() {
    local binary=$1
    local source="$BUILD_DIR/$binary"
    local target="$INSTALL_DIR/$binary"

    if [ ! -f "$source" ]; then
        echo -e "${RED}Error: Binary '$binary' not found in $BUILD_DIR${NC}"
        echo -e "${YELLOW}Please run 'make all' or 'make $binary' to build it first${NC}"
        return 1
    fi

    echo -e "${BLUE}Installing $binary to $INSTALL_DIR...${NC}"
    
    # Try to copy and make executable without sudo
    if cp "$source" "$target" 2>/dev/null; then
        if chmod +x "$target" 2>/dev/null; then
            # Both operations succeeded without sudo
            echo -e "${GREEN}✓ Successfully installed $binary${NC}"
            return 0
        else
            # chmod failed, clean up and try with sudo
            rm -f "$target" 2>/dev/null
        fi
    fi
    
    # Need elevated privileges
    echo -e "${YELLOW}Note: Elevated privileges required for $INSTALL_DIR${NC}"
    if sudo cp "$source" "$target"; then
        if sudo chmod +x "$target"; then
            echo -e "${GREEN}✓ Successfully installed $binary${NC}"
            return 0
        else
            # Clean up on failure
            sudo rm -f "$target" 2>/dev/null
            echo -e "${RED}✗ Failed to make $binary executable${NC}"
            return 1
        fi
    else
        echo -e "${RED}✗ Failed to copy $binary${NC}"
        return 1
    fi
}

# Main installation logic
main() {
    local binaries_to_install=()

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -d|--dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -l|--list)
                list_binaries
                exit 0
                ;;
            -*)
                echo -e "${RED}Error: Unknown option: $1${NC}"
                echo "Run './install.sh --help' for usage information"
                exit 1
                ;;
            *)
                binaries_to_install+=("$1")
                shift
                ;;
        esac
    done

    # If no binaries specified, install kue
    if [ ${#binaries_to_install[@]} -eq 0 ]; then
        binaries_to_install=("kue")
    fi

    # Verify BUILD_DIR exists
    if [ ! -d "$BUILD_DIR" ]; then
        echo -e "${RED}Error: Build directory '$BUILD_DIR' not found${NC}"
        echo -e "${YELLOW}Please run 'make kue' to build the binary first${NC}"
        exit 1
    fi

    # Verify INSTALL_DIR exists or can be created
    if [ ! -d "$INSTALL_DIR" ]; then
        echo -e "${YELLOW}Installation directory '$INSTALL_DIR' does not exist${NC}"
        echo -e "${YELLOW}Attempting to create it...${NC}"
        if mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            echo -e "${GREEN}✓ Created $INSTALL_DIR${NC}"
        else
            echo -e "${YELLOW}Note: Elevated privileges required to create $INSTALL_DIR${NC}"
            if ! sudo mkdir -p "$INSTALL_DIR" 2>/dev/null; then
                echo -e "${RED}Error: Cannot create installation directory${NC}"
                exit 1
            fi
            echo -e "${GREEN}✓ Created $INSTALL_DIR${NC}"
        fi
    fi

    echo -e "${BLUE}=== Kue Installation ===${NC}"
    echo -e "${BLUE}Installation directory: $INSTALL_DIR${NC}"
    echo ""

    # Install each binary
    local success_count=0
    local fail_count=0

    for binary in "${binaries_to_install[@]}"; do
        # Check if binary is valid
        if [[ ! " ${BINARIES[@]} " =~ " ${binary} " ]]; then
            echo -e "${RED}Error: Unknown binary '$binary'${NC}"
            echo -e "${YELLOW}Run './install.sh --list' to see available binaries${NC}"
            fail_count=$((fail_count + 1))
            continue
        fi

        if install_binary "$binary"; then
            success_count=$((success_count + 1))
        else
            fail_count=$((fail_count + 1))
        fi
        echo ""
    done

    # Summary
    echo -e "${BLUE}=== Installation Summary ===${NC}"
    echo -e "${GREEN}Successful: $success_count${NC}"
    if [ $fail_count -gt 0 ]; then
        echo -e "${RED}Failed: $fail_count${NC}"
    fi
    echo ""

    if [ $success_count -gt 0 ]; then
        echo -e "${GREEN}✓ Installation completed!${NC}"
        echo -e "${YELLOW}Make sure $INSTALL_DIR is in your PATH${NC}"
        
        # Check if INSTALL_DIR is in PATH
        if [[ ":$PATH:" == *":$INSTALL_DIR:"* ]]; then
            echo -e "${GREEN}✓ $INSTALL_DIR is already in your PATH${NC}"
        else
            echo -e "${YELLOW}⚠ $INSTALL_DIR is not in your PATH${NC}"
            echo -e "${YELLOW}Add it by running: export PATH=\"$INSTALL_DIR:\$PATH\"${NC}"
        fi
    fi

    if [ $fail_count -gt 0 ]; then
        exit 1
    fi
}

# Run main function
main "$@"
