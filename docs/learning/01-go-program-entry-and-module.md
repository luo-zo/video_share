# 第 1 点：`go run ./cmd/api` 如何找到并启动 Go 程序

这篇笔记记录 `video_share` 项目代码学习的第 1 点，只讨论以下问题：

- `go run ./cmd/api` 到底运行了什么；
- 多个 `.go` 文件如何组成一个包；
- `go.mod` 中的 `module video_share` 有什么作用；
- `package main` 和 `func main()` 分别负责什么；
- 为什么严格来说，部分初始化代码会在 `main()` 之前执行。

## 一、项目中的实际位置

当前后端目录可以简化成：

```text
backend/
├── go.mod
├── cmd/
│   └── api/
│       └── main.go
└── internal/
    ├── config/
    ├── database/
    ├── server/
    └── token/
```

启动命令是：

```powershell
cd C:\Users\27339\Desktop\video_share\backend
go run ./cmd/api
```

其中：

```text
go run        ./cmd/api
运行 Go 程序    要运行的包目录
```

`./cmd/api` 指的是目录，不是某一个具体文件。

## 二、我一开始的理解

> `go run` 一个目录时，Go 会寻找可以执行的 Go 文件。如果目录中有多个 `package main` 文件，它们会合成一个 `main.go` 执行。`go.mod` 开头的 `module video_share` 就是 `go.mod` 当前所在的目录，因此在其他文件夹中引入包时，不用写很长的本地路径，使用模块名就可以。`func main()` 是程序入口，程序从这里开始运行。

这个理解已经抓住了主要流程。下面对其中几个说法作更准确的补充。

## 三、`go run ./cmd/api` 运行的是包

更准确的说法是：

> Go 会读取 `cmd/api` 目录中符合当前构建条件的普通 `.go` 文件，把它们作为一个包编译，然后运行这个包。

假设目录以后变成：

```text
cmd/api/
├── main.go
├── logger.go
├── migrate.go
└── main_test.go
```

正常执行 `go run ./cmd/api` 时：

- `main.go`、`logger.go` 和 `migrate.go` 会参与正常编译；
- `main_test.go` 以 `_test.go` 结尾，只在执行 `go test` 时使用；
- Go 不会因为运行 `cmd/api`，就自动递归编译它下面的所有子目录；
- 其他目录中的包需要通过 `import` 进入依赖关系，Go 才会编译它们。

所以，不应把它理解成“Go 在目录里寻找某个可以执行的文件”。Go 在这里处理的基本单位是**包**。

## 四、多个文件共同组成一个包

如果同一个目录中的几个普通 Go 文件都声明：

```go
package main
```

这些文件会共同组成一个 `main` 包，但不会真的被合并成一个新的 `main.go` 文件。

例如：

```go
// main.go
package main

func main() {
    log := newLogger()
    runServer(log)
}
```

```go
// logger.go
package main

func newLogger() string {
    return "logger"
}
```

```go
// server.go
package main

func runServer(log string) {
}
```

编译器看到的关系可以理解为：

```text
main 包
├── main()
├── newLogger()
└── runServer()
```

因此，`main.go` 可以直接调用另外两个文件中的函数，不需要导入 `logger.go` 或 `server.go`，也不需要在函数名前写文件名。

这里需要记住两条规则：

```text
可以有多个声明为 package main 的文件
整个 main 包只能定义一个 func main()
```

如果两个文件都定义了 `func main()`，编译器会报告名称重复。

## 五、`go.mod` 声明模块的逻辑路径

项目的 [`backend/go.mod`](../../backend/go.mod) 开头是：

```go
module video_share
```

它把逻辑模块路径 `video_share` 与 `go.mod` 所在的 `backend` 目录关联起来：

```text
逻辑模块路径：video_share
模块根目录：  backend/
```

因此会形成以下映射：

| Go 导入路径 | 项目中的实际位置 |
|---|---|
| `video_share` | `backend/` |
| `video_share/internal` | `backend/internal/` |
| `video_share/internal/database` | `backend/internal/database/` |

例如，[`main.go`](../../backend/cmd/api/main.go) 中写着：

```go
import "video_share/internal/database"
```

Go 可以把它拆成：

```text
video_share + internal/database
模块路径        模块内部路径
```

然后找到：

```text
backend/internal/database
```

`video_share` 是开发者在 `go.mod` 中声明的逻辑路径，不是 Go 根据 Windows 文件夹名称自动生成的。

例如，项目即使移动到：

```text
D:\study\my_project\backend
```

只要 `go.mod` 仍然声明：

```go
module video_share
```

项目内部仍然可以这样导入：

```go
import "video_share/internal/database"
```

实际目录名称与模块名称可以不同。发布到公共代码仓库的项目通常会使用完整仓库地址作为模块路径，例如：

```go
module github.com/luo-zo/video_share
```

如果以后修改模块路径，项目内部以 `video_share/` 开头的导入路径也要一起修改。

## 六、为什么不使用 Windows 绝对路径导入

Go 代码不会这样导入项目包：

```go
import "C:/Users/27339/Desktop/video_share/backend/internal/database"
```

它使用模块的逻辑导入路径：

```go
import "video_share/internal/database"
```

这样把项目复制到另一台电脑或另一个磁盘后，代码中的导入路径不需要跟着电脑目录变化。

因此，模块路径的作用不只是“缩短路径”。它为项目提供了一个稳定的逻辑名称，并帮助 Go 定位项目内部的包。

## 七、包名和文件名不能混淆

`backend/internal/database` 中有多个文件，例如：

```text
internal/database/
├── mysql.go
├── migrate.go
├── mysql_test.go
└── migrate_integration_test.go
```

普通构建会使用其中符合构建条件、并且不以 `_test.go` 结尾的文件。这些普通文件声明：

```go
package database
```

当 `main.go` 导入：

```go
import "video_share/internal/database"
```

代码使用的是包名 `database`。所以，即使 `Open` 函数写在 [`mysql.go`](../../backend/internal/database/mysql.go) 中，调用方式仍然是：

```go
database.Open(...)
```

而不是：

```go
database.mysql.Open(...)
```

Go 导入的是包所在的目录，不是某一个源文件。文件名主要帮助开发者整理代码，不会成为函数调用路径的一部分。

## 八、`package main` 与 `func main()`

一个可执行程序需要同时具备：

```go
package main

func main() {
}
```

两者分工如下：

```text
package main → 声明这是一个准备生成可执行程序的包
func main()  → 提供这个程序的主入口
```

只有 `package main`，却没有 `func main()`，程序没有可以进入的主函数。

只有一个名叫 `main` 的函数，但它不在 `package main` 中，它也不会成为可执行程序的入口。

项目当前的入口位于 [`backend/cmd/api/main.go`](../../backend/cmd/api/main.go#L21)：

```go
func main() {
    log := newLogger()
    // 后续启动流程……
}
```

进入 `main()` 后，当前项目首先调用：

```go
log := newLogger()
```

函数只是定义在代码中时不会自动执行。程序运行到调用语句后，`newLogger()` 才会执行。

## 九、`main()` 之前还有初始化过程

把 `func main()` 理解为“程序入口”是正确的。更完整的启动顺序是：

```text
初始化被导入的依赖包
        ↓
初始化当前包的包级变量
        ↓
执行当前包的 init() 函数
        ↓
调用 main.main()
```

例如：

```go
package main

import "fmt"

var name = loadName()

func loadName() string {
    fmt.Println("初始化包级变量")
    return "video_share"
}

func init() {
    fmt.Println("执行 init")
}

func main() {
    fmt.Println("执行 main")
}
```

它的输出顺序会是：

```text
初始化包级变量
执行 init
执行 main
```

因此，`main()` 是主程序入口和主要业务流程的起点，但依赖包初始化、包级变量初始化和 `init()` 会先执行。

## 十、完整执行关系

当前启动过程可以简化为：

```text
执行 go run ./cmd/api
        ↓
Go 定位 backend/go.mod
        ↓
读取 module video_share
        ↓
把 cmd/api 中的普通 Go 文件组成 main 包
        ↓
根据 import 找到并编译依赖包
        ↓
完成依赖包和当前包的初始化
        ↓
调用唯一的 func main()
        ↓
main() 开始组织日志、配置、数据库和 HTTP 服务
```

## 十一、本点结论

执行 `go run ./cmd/api` 时，Go 会把 `cmd/api` 目录中符合构建条件的普通 Go 文件作为一个包编译。多个声明为 `package main` 的文件共同组成一个 `main` 包，并共享其中定义的函数，但它们不会被合并成一个真正的 `main.go` 文件。可执行程序需要 `package main`，并且整个包只能定义一个 `func main()`。

Go 通过 `go.mod` 中的 `module video_share`，把逻辑模块路径 `video_share` 与 `go.mod` 所在的 `backend` 目录关联起来。因此，`video_share/internal/database` 可以定位到模块中的 `internal/database` 包，无需使用电脑上的绝对路径。

Go 完成依赖包、包级变量和 `init()` 初始化后，会调用 `func main()`。从这里开始，`main()` 负责组织和控制程序的主要执行流程。
