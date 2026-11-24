# LearnLinux (`learn-linux`)

English | [中文文档](doc/README_zh.md)

## Overview

**LearnLinux** is a zero-dependency, single-binary interactive Linux CLI trainer written in Go. With a built-in in-memory virtual filesystem (VFS) and a Bash-compatible virtual shell, you can safely practice core Linux commands on **macOS, Windows, and Linux** without VMs, WSL, or Docker.

- **100% Safe In-Memory Sandbox**: All file operations, permissions, and builds run strictly in memory—never touching your host files.
- **8 Practical Chapters + Free Sandbox**: Hands-on lessons with live objective verification, `hint`, and instant `reset`.
- **Authentic Terminal Experience**: Tab completion, command history (`↑` / `↓`), pipes, redirections, wildcards, command chaining, plus built-in `less` and `nano`.
- **Bilingual UI & Concise `man` Pages**: Switch between English and Simplified Chinese anytime via `lang en` / `lang zh` (56 built-in command manuals).

## Quick Start

Pre-built, ready-to-run binaries for macOS, Linux, and Windows are available on the [GitHub Releases](https://github.com/MuteLoc0/learn-linux/releases) page—download the binary for your platform and run it directly in your terminal.

Progress, language preference, and command history are automatically saved under `~/.learn-linux-cli`.

## How to Build

Requires Go 1.27+ (matching [`go.mod`](go.mod)).

```bash
# Build and run
make build
./learn-linux

# Or run directly with Go
go run ./cmd/learn-linux

# Run tests & build multi-platform release binaries
make test
make dist
```
