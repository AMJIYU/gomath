# 集中测试

所有测试源码、基准测试、参考数据和开发验证脚本都放在这个目录。

- `_cases/<包路径>/`：原包的 Go 测试；根包放在 `_cases/root/`。
- `data/<包路径>/`：固定输入、JSON、参考图片等测试数据。
- `reference/`：开发阶段的独立参考生成脚本，不参与产品运行。
- `cmd/run/`、`internal/harness/`：统一入口和 Go source overlay 工具。
- `suite_test.go`：让通常的全项目测试命令执行整个集中测试集。

```sh
# 完整测试；也检查目录规则及原包测试中的 vet 问题
CGO_ENABLED=0 go test -tags noasm ./...

# 指定包；可重复 -pkg，根包使用 root
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -pkg=render -count=1

# 指定测试、基准测试和竞态检测
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -run TestPinnedManimIntermediateBehavior
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^$' -bench . -benchmem
go run ./tests/cmd/run -pkg=three -race
```

工具使用 Go 自带的 `-overlay` 将源码虚拟映射到原包，以保留私有接口和
`internal` 包的测试能力。不会在产品目录中生成测试文件。
`_cases` 前缀让普通包发现跳过这些原始源码，避免包名混用和重复执行。
顶层测试入口会读取源码、嵌入资源和参考数据，让它们的变更使测试缓存失效。

按名称筛选或运行 benchmark 时请用统一入口；直接对产品包执行 `go test`
不包含这些虚拟测试。不要使用 `go test ./... -run 某个业务测试名` 来筛选，
因为这会同时跳过顶层的 `TestCentralSuite`。
