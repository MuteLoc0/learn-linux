# LearnLinux (`learn-linux`)

[English](../README.md) | 中文文档

## 中文说明

**LearnLinux** 是一款用 Go 编写的零依赖、单二进制交互式 Linux 命令行学习工具。内置内存虚拟文件系统（VFS）与兼容 Bash 语法的虚拟 Shell，无需虚拟机、WSL 或 Docker，即可在 **macOS / Windows / Linux** 终端里零风险练习核心 Linux 命令。

- **100% 安全沙盒**：所有文件操作、权限修改与编译均在纯内存中运行，绝不触碰宿主机真实文件。
- **8 章实战课程 + 自由沙盒**：提供实时目标校验、`hint` 提示与一键 `reset` 重置。
- **完整终端交互**：支持 Tab 补全、历史命令（`↑` / `↓`）、管道、重定向、通配符、命令链，以及内置 `less` 和 `nano`。
- **中英双语 & 精简 `man` 手册**：输入 `lang en` / `lang zh` 实时切换语言，内置 56 条常用命令手册。

## 快速开始

前往 [GitHub Releases](https://github.com/MuteLoc0/learn-linux/releases) 下载对应平台的可执行文件后，在终端中执行：

- **macOS（Apple Silicon / Intel）**：
  ```bash
  chmod +x ./learn-linux-darwin-arm64   # Intel Mac 请使用 learn-linux-darwin-amd64
  ./learn-linux-darwin-arm64
  ```
  > **macOS 安全提示**：首次运行未签名程序时，系统可能提示“未打开……Apple 无法验证是否包含恶意软件”。请前往 **系统设置 → 隐私与安全性 → 安全性**，点击 **仍然允许**（或执行 `xattr -d com.apple.quarantine ./learn-linux-darwin-arm64`），然后重新运行即可。也建议 macOS 用户直接参考下方 [如何编译](#如何编译) 自行编译运行，可避免该安全拦截。
- **Linux（x86_64）**：
  ```bash
  chmod +x ./learn-linux-linux-amd64
  ./learn-linux-linux-amd64
  ```
- **Windows（x86_64）**：
  ```powershell
  .\learn-linux-windows-amd64.exe
  ```

学习进度、语言偏好与命令历史等存档文件保存在 `~/.learn-linux-cli` 目录中。

## 如何编译

前置要求：安装 Go 1.27+（参见 [`go.mod`](../go.mod)）。

```bash
# 编译并运行
make build
./learn-linux

# 或直接使用 Go 运行
go run ./cmd/learn-linux

# 运行测试 & 构建多平台发行版
make test
make dist
```
