<div align="center">

[English](README.md) | **简体中文**

# agentsmd

**你的 AI Agent 的说明书，值得拥有 CI。**

`agentsmd` 是一个单二进制工具集，让 `AGENTS.md` 及其周边的一切保持诚实：
它基于仓库**真实的工具链生成**有据可查的 AGENTS.md，**校验**文档中提到的
每条命令、每个文件是否真实存在，**审计**质量与 token 成本，并用一行
import 把 Claude Code / Gemini CLI **桥接**到 AGENTS.md。

[![CI](https://github.com/youwei792/agentsmd/actions/workflows/ci.yml/badge.svg)](https://github.com/youwei792/agentsmd/actions/workflows/ci.yml)
[![Go Reference](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Zero dependencies](https://img.shields.io/badge/dependencies-0-brightgreen)](#设计原则)

![agentsmd doctor — 在它自己的仓库上自评 100/100](docs/demo.svg)

</div>

---

## 为什么需要它

现在每个正经仓库都要给 AI 编码 Agent 写说明文件。[AGENTS.md](https://agents.md)
已成为 Linux Foundation 采纳的跨工具标准——Codex、Cursor、Gemini CLI、
Copilot、Jules、Amp 等 30+ 工具原生读取。但是：

- **这份文件会说谎。** 有人把 `pnpm` 换成了 `npm`、改了脚本名、挪了文件——
  你团队里的每个 Agent 就开始照着过期指令"一本正经地幻觉"。没有任何东西
  能发现这件事，因为 AGENTS.md *从不被执行*。
- **Claude Code 不读 AGENTS.md。** 它是唯一的钉子户，只读 `CLAUDE.md`。
  大家用 symlink 绕（Windows 上会坏、部分工具会双读），或者手工复制内容
  （一周内必然漂移）。
- **没人管理 token 预算。** AGENTS.md 会被装进每一个 Agent 会话。一个臃肿
  的 6k-token 文件是对每个任务的永久税。

agentsmd 把 Agent 指令文件当代码对待：**被检查、被度量、被自动同步。**

## 安装

```bash
# Go
go install github.com/youwei792/agentsmd@latest

# Homebrew（Linux 也可用）
brew install youwei792/tap/agentsmd

# npm（全局安装）
npm install -g @momo792/agentsmd

# 或到 Releases 下载二进制（linux/darwin/windows，amd64/arm64）
```

## 使用

### 1. 生成有据可查的 AGENTS.md

```bash
agentsmd init
```

不是模板——agentsmd 检测你的包管理器、脚本、Makefile 目标、框架、测试
运行器、linter、monorepo 结构和 CI 命令，然后只写它**能证明存在**的内容。
检测不到的，留成明确的 `TODO` 给你填。

### 2. 让它保持真实——放进 CI

```bash
agentsmd check
```

递归解析根目录和子目录中的 Agent 指令文件（代码块**和**行内反引号），提取命令与文件引用，对照
仓库现实逐一核验：npm/pnpm/yarn 脚本、Makefile 目标、just 配方、
`go test ./...` 路径、pytest 目标、compose 文件、requirements 文件、
`./scripts/foo.sh`、死链——并给出相近匹配提示（`pnpm testt` → 你是想写
`test` 吗？）。

**保守是设计出来的**：不确定某条引用真的坏了，它就闭嘴。零误报就是
这个产品的全部。

一步接入 CI——仓库根目录本身就是一个 [composite GitHub Action](action.yml)：

```yaml
- uses: youwei792/agentsmd@v1
  with:
    strict: true
```

### 3. 质量审计与评分

```bash
agentsmd lint
```

规则包括：token 膨胀（含占上下文窗口的比例）、缺少构建/测试命令、死引用、
包管理器错配（文档写 `yarn`、lockfile 是 `pnpm`）、无法执行的含糊规则、
遗留 TODO、重复章节、以及相对于 manifests 的过期程度。A–F 评分，
CI 友好的退出码。

### 4. 桥接 Claude Code 与 Gemini CLI

```bash
agentsmd sync
```

向 `CLAUDE.md`/`GEMINI.md` 写入一行 `@AGENTS.md` import——这是 Anthropic
推荐的方式——而不是 symlink。另有 `--mode copy` 与 `--mode symlink`；
copy 模式拒绝碰它不管理的文件。`agentsmd sync --check` 在桥接过期时让
CI 变红。

### 5. 看清你的上下文预算

```bash
agentsmd tokens
```

汇总每个 Agent 指令文件的 token 成本，显示它吃掉了 128k/200k/1M 上下文
窗口的百分之几。

### 6. 一次全做

```bash
agentsmd doctor
```

## 命令一览

| 命令 | 作用 |
|---|---|
| `agentsmd init` | 从检测到的仓库事实生成 AGENTS.md（`--minimal`、`--force`、`--dry-run`） |
| `agentsmd check` | 校验每条命令/文件引用真实存在（`--strict`、`--json`） |
| `agentsmd lint` | 质量审计 + A–F 评分（`--json`） |
| `agentsmd tokens` | Agent 文件的上下文成本（`--json`） |
| `agentsmd sync` | 把 CLAUDE.md/GEMINI.md 桥接到 AGENTS.md（`--mode import\|copy\|symlink`、`--check`） |
| `agentsmd doctor` | 上述全部，一份报告（`--json`） |
| `agentsmd skills` | 校验 Agent Skills（`SKILL.md`）包——frontmatter 规范、bundle 完整性、token 成本 |
| `agentsmd org` | 舰队报告：一个组织/用户所有公开仓库的 AGENTS.md 健康度（需 `gh`） |
| `agentsmd analyze` | 显示检测到的工具链事实（`--json`） |

表格中标注了 `--json` 的审计和检查命令支持机器可读输出，方便搭建你自己的看板。

## 它能检测什么

包管理器（`packageManager` 字段 + lockfile）、npm/pnpm/yarn/bun
workspaces、go.work、Cargo workspace、package.json 脚本、Makefile 目标、
justfile 配方、Poetry/uv/pip 配置、pytest/ruff/eslint/prettier/biome/
golangci-lint/clippy 配置、来自依赖清单的 60+ 框架、GitHub Actions 与
GitLab CI 命令、Docker，以及你已有的 Agent 文件。

## 安全姿态

Agent 指令文件是一个真实的攻击面：Agent 会逐字执行里面的命令，而人们
喜欢把密钥直接粘进去。所以 `lint` 内置了安全规则：

- **`SECRETS-FOUND`** —— 指令文件中出现的真实 API key、GitHub/Slack
  token、AWS key id、私钥块（`sk-xxx…` 这类占位符和 AWS `…EXAMPLE` 规范
  示例保持静默）。
- **`RISKY-COMMAND`** —— 被写成"Agent 应该执行"的 `curl … | sh`、
  `sudo`、`eval`、`chmod 777`、`rm -rf ~`。

工具本身的设计目标就是可以在任何地方放心运行：

- **从不执行**它读到的命令——只做解析和 `os.Stat`。
- **完全离线**（唯一例外是 `org`，它调用 `gh` CLI）。
- **零依赖**、无遥测、只检查仓库检出内的文件——逃逸仓库根的引用
  一律不读。
- **发布带校验和**：GitHub Action 在运行二进制前会先验证
  `checksums.txt`。

细节与漏洞报告：[SECURITY.md](SECURITY.md)。公开的准确性证据：
[docs/benchmarks.md](docs/benchmarks.md)。

## 设计原则

1. **保守或沉默。** 一个狼来了的检查器会被卸载。每条发现都必须可证明。
   引擎在真实生产 AGENTS.md 上做过验证（见
   [docs/benchmarks.md](docs/benchmarks.md)）：8 个仓库、约 555 个引用，
   早期版本的每一条发现都经过人工甄别，8 类误报在 v0.1.1 全部修复并
   带回归测试。
2. **生成有据。** `init` 只写它在仓库里真实检测到的命令，绝不发明一个
   不存在的 `make test`。
3. **零依赖。** 纯标准库 Go（约 5k 行）。`go build` 就是全部供应链。
   安全团队一个下午能读完每一行。
4. **CI 优先。** 所有命令都有明确退出码，审计和检查命令支持 `--json`；仓库根目录*就是*那个
   GitHub Action。
5. **你的文件你做主。** `sync` 的 copy 模式拒绝碰它不管理的文件；
   `init` 替换前先备份。

## symlink 的坑

给 Claude Code 的流行方案是 `ln -s AGENTS.md CLAUDE.md`。它能用，直到
不能用：Windows 检出（未开开发者模式）会把 symlink 物化成一份拷贝
（立即漂移），部分工具会把它读两遍或直接读懵，Windows 上的 git symlink
还需要额外配置。agentsmd 默认的 import 模式只是一个任何工具都能读的
三行小文件：

```markdown
<!-- managed by agentsmd: this file bridges to AGENTS.md. Edit AGENTS.md instead. -->

@AGENTS.md
```

## 本仓库吃自己的狗粮

`dogfood` CI 任务在每次 push 时对本仓库自己的 AGENTS.md 运行
`agentsmd check .`——`doctor` 自评 100/100。一旦某条文档化的命令坏掉，
CI 会在任何 Agent 注意到之前先变红。

## 路线图

- [x] `agentsmd skills` — SKILL.md Agent Skills 校验（v0.2.0）
- [x] 组织模式：`agentsmd org <gh-org>` 跨仓库健康报告（v0.2.0）
- [ ] `--fix` 安全自动修复（死链 → 相近匹配）
- [ ] pre-commit 钩子：manifest 变更时自动检查
- [x] npm 分发已上线：`npm install -g @momo792/agentsmd`（esbuild 式平台包；无作用域的 `agentsmd` 名字被 npm 防typosquat机制拦截，后续可向 npm 官方申请解封）

欢迎 PR——见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

[MIT](LICENSE) © youwei792
