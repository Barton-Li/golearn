package gee

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
)

// trace 函数用于获取并返回触发 panic 时的堆栈信息。
// 参数 message 为 panic 时附加的信息。
// 返回值为拼接了 panic 信息和堆栈跟踪的字符串。
func trace(message string) string {
    var pcs [32]uintptr
		// 获取当前调用栈的信息，存储到pcs中
    n := runtime.Callers(3, pcs[:]) // 获取调用 trace 函数的调用栈信息
		// 创建一个strings.Builder对象str，用于构建返回的字符串。
    var str strings.Builder
    str.WriteString(message + "\nTraceback:") // 开始构建返回的字符串，包含 panic 信息和 traceback 标题
    for _, pc := range pcs[:n] { // 遍历调用栈信息
        fn := runtime.FuncForPC(pc) // 获取函数信息
        file, line := fn.FileLine(pc) // 获取文件和行号信息
        str.WriteString(fmt.Sprintf("\n%s:%d ", file, line)) // 将文件和行号添加到字符串中
    }
    return str.String() // 返回构建完成的字符串
}

// Recovery 函数返回一个处理程序（Handlerfunc），该处理程序用于recover从HTTP请求处理中引发的panic，并返回500错误。
// 返回的处理函数类型可直接用于类似Gin或Echo等HTTP框架的路由处理中。
func Recovery() Handlerfunc {
    return func(c *Context) {
        defer func() { // 使用defer确保在panic时执行以下逻辑
            if err := recover(); err != nil { // 捕获并处理panic
                message := fmt.Sprintf("%s", err) // 将panic的内容转换为字符串
                log.Printf("%s\n\n", trace(message)) // 使用trace函数获取堆栈信息并记录到日志
                c.Fail(http.StatusInternalServerError, message) // 向客户端返回500错误信息
            }
        }()

        c.Next() // 继续执行后续的处理函数

    }
}