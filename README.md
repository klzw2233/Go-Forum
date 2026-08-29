# Go-Forum

[![CI](https://github.com/klzw2233/Go-Forum/actions/workflows/ci.yml/badge.svg)](https://github.com/klzw2233/Go-Forum/actions/workflows/ci.yml)

熟人封闭传统 BBS。一个站点、一群互相认识的人。未登录什么都看不见。

领域语言见 [CONTEXT.md](CONTEXT.md)。第一刀只做发帖闭环，范围见 [docs/adr/0002-v1-scope.md](docs/adr/0002-v1-scope.md)。

## 第一刀能做什么

配置启动 → 创始人登录 → 建版块 → 开主题（标题 + 一楼 Markdown）→ 回帖 → 立刻看见。Markdown 里的 `http://` / `https://` 图床地址会当图显示。

## 第一刀不能做什么

邀请码、注册、隐藏帖、编辑历史、置顶、挪版、停用、升运营者、改别人密码、搜索、未读、通知、私信。创始人是唯一能登录的人。

## 怎么跑

需要 Go 1.22+（本机开发用 1.26）。

```text
copy forum.example.toml forum.toml
```

编辑 `forum.toml`：把 `founder.password` 改成你自己的密码。`forum.toml` 和 `forum.db` 不会进 git。

```text
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/forum
```

推送到 GitHub 后，Actions 会再跑一遍 `gofmt` / `go vet` / `go test`（见 `.github/workflows/ci.yml`）。

浏览器打开 <http://127.0.0.1:8080>，用配置里的登录名和密码登录。

指定配置文件：

```text
go run ./cmd/forum -config forum.toml
```

## 配置项

见 `forum.example.toml`：

| 项 | 含义 |
|---|---|
| `listen` | HTTP 监听地址，默认 `127.0.0.1:8080` |
| `database` | SQLite 文件路径，默认 `forum.db` |
| `founder.login_name` | 创始人登录名（字母开头，只含字母数字下划线） |
| `founder.display_name` | 创始人显示名 |
| `founder.password` | 只在库里还没有这个登录名时用来创建创始人；已存在则不覆盖密码 |

## 仓库结构

```text
cmd/forum/           入口
internal/forum/      领域规则
internal/store/      SQLite
internal/web/        HTTP、模板、Markdown
internal/config/     读 forum.toml
```
