# GitHub Actions CI

仓库：`git@github.com:klzw2233/Go-Forum.git`

工作流文件：`.github/workflows/ci.yml`

## 何时跑

每次 `push`（含功能分支）和每次 `pull_request`。

## 做什么

在 `ubuntu-latest` 上，`CGO_ENABLED=0`：

1. 按 `go.mod` 安装 Go
2. `gofmt -l .` 必须为空
3. `go vet ./...`
4. `go test ./...`

SQLite 用的是 `modernc.org/sqlite`，不需要 CGO。CI 里关掉 CGO，避免误链上系统 sqlite。

## 本地对齐

推送前在本机跑：

```text
gofmt -l .
go vet ./...
go test ./...
```
