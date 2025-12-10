#!/bin/bash
# ============================================================
# llm-mux installer
# https://github.com/nghyane/llm-mux
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nghyane/llm-mux/main/install.sh | bash
#
# Options:
#   --no-service    Skip service installation (binary only)
#   --version VER   Install specific version (default: latest)
#   --dir DIR       Custom install directory
#   --help          Show help
# ============================================================

set -euo pipefail

# --- Configuration -------------------------------------------

REPO="nghyane/llm-mux"
BINARY_NAME="llm-mux"
SERVICE_NAME="com.llm-mux"

VERSION=""
INSTALL_DIR=""
SKIP_SERVICE=false

# --- Utilities -----------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()   { echo -e "${GREEN}==>${NC} $*"; }
info()  { echo -e "${BLUE}   $*${NC}"; }
warn()  { echo -e "${YELLOW}warning:${NC} $*" >&2; }
error() { echo -e "${RED}error:${NC} $*" >&2; exit 1; }

command_exists() { command -v "$1" &>/dev/null; }

# --- Platform Detection --------------------------------------

OS=""
ARCH=""

detect_platform() {
    case "$(uname -s)" in
        Darwin*) OS="darwin" ;;
        Linux*)  OS="linux" ;;
        MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
        *) error "Unsupported OS: $(uname -s)" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)  ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $(uname -m)" ;;
    esac
}

detect_install_dir() {
    [[ -n "$INSTALL_DIR" ]] && return

    if [[ -w "/usr/local/bin" ]]; then
        INSTALL_DIR="/usr/local/bin"
    elif mkdir -p "$HOME/.local/bin" 2>/dev/null; then
        INSTALL_DIR="$HOME/.local/bin"
    else
        error "No writable install directory. Use --dir to specify."
    fi
}

# --- Download & Install --------------------------------------

fetch() {
    local url="$1" output="$2"
    if command_exists curl; then
        curl -fsSL "$url" -o "$output"
    elif command_exists wget; then
        wget -q "$url" -O "$output"
    else
        error "curl or wget required"
    fi
}

get_latest_version() {
    [[ -n "$VERSION" ]] && return

    log "Checking latest version..."
    local response
    response=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" -)
    VERSION=$(echo "$response" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    [[ -z "$VERSION" ]] && error "Failed to get latest version"
}

verify_checksum() {
    local file="$1" checksums="$2"
    local filename expected actual

    filename=$(basename "$file")
    expected=$(grep "$filename" "$checksums" 2>/dev/null | awk '{print $1}')
    [[ -z "$expected" ]] && return 0

    if command_exists sha256sum; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command_exists shasum; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        return 0
    fi

    [[ "$expected" != "$actual" ]] && error "Checksum mismatch for $filename"
    info "Checksum verified ✓"
}

install_binary() {
    local tmp_dir version_num ext archive_name download_url binary_path

    tmp_dir=$(mktemp -d)
    trap "rm -rf '$tmp_dir'" EXIT

    version_num="${VERSION#v}"
    ext="tar.gz"
    [[ "$OS" == "windows" ]] && ext="zip"

    archive_name="llm-mux_${version_num}_${OS}_${ARCH}.${ext}"
    download_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive_name}"

    log "Downloading ${BINARY_NAME} ${VERSION}..."
    fetch "$download_url" "$tmp_dir/$archive_name"
    fetch "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" "$tmp_dir/checksums.txt" 2>/dev/null || true
    verify_checksum "$tmp_dir/$archive_name" "$tmp_dir/checksums.txt"

    log "Installing to ${INSTALL_DIR}..."
    cd "$tmp_dir"
    if [[ "$ext" == "zip" ]]; then
        unzip -q "$archive_name"
    else
        tar -xzf "$archive_name"
    fi

    binary_path=$(find . -name "$BINARY_NAME" -o -name "${BINARY_NAME}.exe" 2>/dev/null | head -1)
    [[ -z "$binary_path" ]] && error "Binary not found in archive"

    if [[ -w "$INSTALL_DIR" ]]; then
        cp "$binary_path" "$INSTALL_DIR/"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"*
    else
        sudo cp "$binary_path" "$INSTALL_DIR/"
        sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"*
    fi
}

# --- Config --------------------------------------------------

init_config() {
    local config_file="$HOME/.config/llm-mux/config.yaml"

    if [[ -f "$config_file" ]]; then
        info "Config exists: $config_file"
        return
    fi

    log "Creating config..."
    "$INSTALL_DIR/$BINARY_NAME" --init 2>/dev/null || true
}

# --- Service: macOS (launchd) --------------------------------

service_macos_plist() {
    cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${SERVICE_NAME}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${HOME}/.local/var/log/llm-mux.log</string>
    <key>StandardErrorPath</key>
    <string>${HOME}/.local/var/log/llm-mux.log</string>
</dict>
</plist>
EOF
}

service_macos_install() {
    local plist_dir="$HOME/Library/LaunchAgents"
    local plist_path="$plist_dir/${SERVICE_NAME}.plist"
    local log_dir="$HOME/.local/var/log"

    log "Setting up launchd service..."

    mkdir -p "$plist_dir" "$log_dir"
    service_macos_plist > "$plist_path"

    launchctl unload "$plist_path" 2>/dev/null || true
    launchctl load "$plist_path"

    info "Service installed: $plist_path"
}

service_macos_status() {
    launchctl list | grep -q "$SERVICE_NAME" && echo "running" || echo "stopped"
}

# --- Service: Linux (systemd) --------------------------------

service_linux_unit() {
    cat <<EOF
[Unit]
Description=llm-mux - Multi-provider LLM gateway
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
}

service_linux_install() {
    local service_dir="$HOME/.config/systemd/user"
    local service_path="$service_dir/llm-mux.service"

    log "Setting up systemd service..."

    mkdir -p "$service_dir"
    service_linux_unit > "$service_path"

    systemctl --user daemon-reload
    systemctl --user enable llm-mux 2>/dev/null
    systemctl --user start llm-mux

    info "Service installed: $service_path"
}

service_linux_status() {
    systemctl --user is-active llm-mux 2>/dev/null || echo "stopped"
}

# --- Service: Router -----------------------------------------

setup_service() {
    case "$OS" in
        darwin) service_macos_install ;;
        linux)  service_linux_install ;;
        *)      warn "Service not supported on $OS" ;;
    esac
}

# --- Output --------------------------------------------------

print_success() {
    local status="(not running)"

    if [[ "$SKIP_SERVICE" != "true" ]]; then
        case "$OS" in
            darwin) [[ $(service_macos_status) == "running" ]] && status="running ✓" ;;
            linux)  [[ $(service_linux_status) == "active" ]] && status="running ✓" ;;
        esac
    fi

    echo ""
    echo -e "${GREEN}llm-mux ${VERSION} installed successfully!${NC} [${status}]"
    echo ""
    echo "Next steps:"
    echo "  1. Login to a provider:"
    echo "     llm-mux --login              # Gemini"
    echo "     llm-mux --claude-login       # Claude"
    echo "     llm-mux --copilot-login      # GitHub Copilot"
    echo ""

    if [[ "$SKIP_SERVICE" == "true" ]]; then
        echo "  2. Start the server:"
        echo "     llm-mux"
    else
        echo "  2. Service commands:"
        case "$OS" in
            darwin)
                echo "     launchctl stop $SERVICE_NAME   # Stop"
                echo "     launchctl start $SERVICE_NAME  # Start"
                ;;
            linux)
                echo "     systemctl --user stop llm-mux   # Stop"
                echo "     systemctl --user start llm-mux  # Start"
                ;;
        esac
    fi

    echo ""
    echo "  3. Use the API:"
    echo "     curl http://localhost:8318/v1/chat/completions \\"
    echo "       -H 'Content-Type: application/json' \\"
    echo "       -d '{\"model\": \"gemini-2.5-flash\", \"messages\": [...]}'"
    echo ""

    # PATH warning
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        warn "$INSTALL_DIR is not in PATH"
        echo "  Add to PATH: export PATH=\"$INSTALL_DIR:\$PATH\""
        echo ""
    fi
}

# --- Help ----------------------------------------------------

usage() {
    cat <<EOF
llm-mux installer

Usage:
    curl -fsSL https://raw.githubusercontent.com/nghyane/llm-mux/main/install.sh | bash

Options:
    --no-service        Skip service setup (install binary only)
    --version VERSION   Install specific version (default: latest)
    --dir DIRECTORY     Install to custom directory
    -h, --help          Show this help

Examples:
    # Default install (binary + service)
    curl -fsSL .../install.sh | bash

    # Binary only, no service
    curl -fsSL .../install.sh | bash -s -- --no-service

    # Specific version
    curl -fsSL .../install.sh | bash -s -- --version v1.0.0
EOF
    exit 0
}

# --- Main ----------------------------------------------------

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --no-service)   SKIP_SERVICE=true; shift ;;
            --version)      VERSION="$2"; shift 2 ;;
            --dir)          INSTALL_DIR="$2"; shift 2 ;;
            -h|--help)      usage ;;
            *)              error "Unknown option: $1" ;;
        esac
    done
}

main() {
    parse_args "$@"

    echo ""
    log "llm-mux installer"
    echo ""

    detect_platform
    info "Platform: $OS/$ARCH"

    detect_install_dir
    get_latest_version
    install_binary
    init_config

    if [[ "$SKIP_SERVICE" != "true" ]]; then
        setup_service
    fi

    print_success
}

main "$@"
