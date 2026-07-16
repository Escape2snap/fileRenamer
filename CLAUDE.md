# CLAUDE.md — fileRenamer 项目规范

## 构建与测试

```bash
go build -o fileRenamer .      # 编译
go vet ./...                    # 静态检查
go build -o /dev/null .         # 验证编译（不输出）
```

## 代码风格

- Go 标准格式：`gofmt` / `go fmt`
- 错误处理不省略 `err != nil`
- 导出的类型和函数写 godoc 注释
- 日志分级：`logVerbose()`（仅在 `--verbose` 时输出）、`logError()`
- 哈希算法枚举在 `hasher.go` 的 `HashAlgo` 中维护，新增算法需同步更新：
  - `ParseHashConfig()` 的 switch
  - `newHasher()` 的 switch
  - `FullHexLen()` 的 switch
  - `normaliseAlgoName()` 的别名规则

## 碰撞处理逻辑

1. 主哈希（可配置）相同的两个文件进入碰撞流程
2. sha3-512 相同 → **真重复** → 删除创建时间更晚的文件（`resolveCollision`）
3. sha3-512 不同 → **真哈希碰撞**（极小概率）→ stderr 告警，保留原文件不动
4. `--force` 跳过 sha3-512 检查，直接删除新文件

## 提交规范

使用 Conventional Commits 风格：

```
feat:     新功能
fix:      Bug 修复
docs:     文档（README、CLAUDE.md）
style:    代码格式（不影响逻辑）
refactor: 重构（不修 bug 不加功能）
test:     测试
chore:    构建、依赖、配置
```

格式：
```
<type>: <简短描述>

<可选详细说明>
```

示例：
```
feat: 添加 SHA3-224 截断模式支持
fix: 修正 short truncation warning 对 -y flag 的判断
docs: 更新 README 去重逻辑说明
```

## 文件说明

| 文件 | 职责 |
|------|------|
| `main.go` | CLI 参数解析、入口编排 |
| `hasher.go` | 哈希算法注册、文件哈希计算 |
| `processor.go` | 文件扫描（glob/regex）、重命名、碰撞检测与解决 |

## 跨平台注意事项

- 使用 `filepath.Join` / `filepath.Dir` 处理路径（非字符串拼接）
- 创建时间用 `github.com/djherbis/times` 库（跨平台）
- 构建时验证所有目标平台：`GOOS=linux GOARCH=amd64` / `GOOS=windows GOARCH=amd64` / `GOOS=darwin GOARCH=arm64`
