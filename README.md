# fileRenamer

> 将文件重命名为内容哈希值，自动检测并删除重复文件。

## 快速开始

```bash
# 处理当前目录所有文件（默认 blake2b — 快速）
./fileRenamer

# 处理所有 JPG 文件
./fileRenamer ./*.jpg

# 递归处理所有 .go 文件
./fileRenamer -r -e '\.go$'

# 用 SHA3-224（默认是 blake2b，加 -hash 切换）
./fileRenamer -hash sha3-224

# 强制删除重复（跳过 sha3-512 二次验证）
./fileRenamer --force *.txt
```

## 安装

```bash
# 本地构建
go build -o fileRenamer .

# 或直接安装到 $GOPATH/bin
go install
```

### 交叉编译

```bash
GOOS=windows GOARCH=amd64 go build -o fileRenamer.exe .
GOOS=darwin  GOARCH=arm64 go build -o fileRenamer .
```

## 支持的哈希算法

| 算法 | CLI 名称（大小写不敏感） | 完整长度（hex） | 速度 |
|------|--------------------------|----------------|------|
| **BLAKE2b** | `blake2b` / `blake2b256` | 64 | ⚡ 最快（默认） |
| BLAKE2b-512 | `blake2b-512` / `blake2b512` | 128 | ⚡ 快 |
| BLAKE2s | `blake2s` | 64 | ⚡ 快（32位友好） |
| SHA3-224 | `sha3-224` | 56 | 🐢 标准 |
| SHA3-256 | `sha3-256` | 64 | 🐢 标准 |
| SHA3-384 | `sha3-384` | 96 | 🐢 标准 |
| SHA3-512 | `sha3-512` | 128 | 🐢 标准 |
| SHA-224 | `sha-224` / `sha224` | 56 | 🐢 标准 |
| SHA-256 | `sha-256` / `sha256` | 64 | 🐢 标准 |
| SHA-384 | `sha-384` / `sha384` | 96 | 🐢 标准 |
| SHA-512 | `sha-512` / `sha512` | 128 | 🐢 标准 |
| SHA-512/224 | `sha-512/224` | 56 | 🐢 标准 |
| SHA-512/256 | `sha-512/256` | 64 | 🐢 标准 |

### 截断模式

在算法名后加 `:N` 取前 N 个 hex 字符：

```bash
./fileRenamer -hash sha3-224:16   # 取前 16 位
./fileRenamer -hash sha256:12     # 取前 12 位
```

**安全警告**：N ≤ 8 时触发碰撞风险确认提示。

## 去重逻辑

1. 计算文件**主哈希**（可配置，默认 blake2b）
2. 若主哈希相同，计算 **sha3-512** 二次验证
3. sha3-512 相同 → 真正重复 → **删除创建时间更晚的文件**
4. sha3-512 不同 → 哈希碰撞（极小概率）→ **告警 + 保留原文件**
5. `--force` → **跳过 sha3-512 检查**，始终删除新文件
6. 创建时间相同时 → 删除后处理的文件

## 完整选项

```
-hash <algo>      哈希算法（默认 "blake2b"）
-force, -f        强制删除碰撞文件
-recursive, -r    递归子目录
-regex, -e        正则匹配（grep 风格）
-quiet, -q        静默模式
-verbose, -v      详细输出
-yes, -y          自动确认提示
-help, -h         帮助
```

## 文件匹配

| 模式 | 示例 | 说明 |
|------|------|------|
| **默认** | `./fileRenamer` | 扫描当前目录所有文件 |
| **通配符** | `./fileRenamer ./*.jpg` | 标准 glob 匹配 |
| **正则** | `./fileRenamer -e '\.txt$'` | grep 风格，第一个参数为 pattern，后续为搜索目录 |
| **正则多目录** | `./fileRenamer -e '\.go$' ./src ./lib` | 同时搜索多个目录 |

## 项目结构

```
fileRenamer/
├── main.go        入口 & CLI 参数
├── hasher.go      哈希算法 & 计算
├── processor.go   文件扫描 & 去重处理
├── go.mod / go.sum
├── README.md
└── CLAUDE.md
```

## 许可证

MIT
