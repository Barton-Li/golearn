package LGRPC

import (
	"fmt"
	"net/http"
	"text/template"
)

const debugText = `<html>
	<body>
	<title>GeeRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

// 初始化debug模板
// 此变量是一个template.Template类型的变量，通过解析debugText文本后创建。
// 使用template.Must函数确保在解析失败时能自动触发panic，简化了错误处理流程。
var debug = template.Must(template.New("RPC debug").Parse(debugText))

type debugHTTP struct {
	*Server
}
type debugService struct {
	Name   string
	Method map[string]*methodType
}

// ServeHTTP 是debugHTTP类型的成员方法，用于处理HTTP请求。
// 该方法遍历服务映射，收集所有服务信息，并通过debug模板渲染这些信息响应客户端。
//
// 参数:
// w http.ResponseWriter - 用于向客户端发送HTTP响应的接口。
// req *http.Request - 表示客户端发起的HTTP请求的结构体指针。
//
// 返回值:
// 无
func (server debugHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 初始化一个空的服务切片，用于存储遍历服务映射得到的服务信息。
	var services []debugService

	// 遍历serviceMap，将每个服务的名称和方法添加到services切片中。
	// 遍历服务映射表，并将每个服务的名称和方法添加到services切片中
	server.serviceMap.Range(func(namei, svci interface{}) bool {
		// 将接口类型转换为具体的服务实例和名称
		svc := svci.(*service)
		services = append(services, debugService{
			Name:   namei.(string), // 服务名称
			Method: svc.method,     // 服务方法
		})
		return true // 继续遍历
	})

	// 使用收集到的服务信息，执行debug模板，并将结果通过HTTP响应返回给客户端。
	err := debug.Execute(w, services)
	if err != nil {
		// 如果在执行模板过程中出现错误，将错误信息作为HTTP响应返回给客户端。
		_, _ = fmt.Fprintln(w, "rpc:error executing debug template:", err.Error())
	}
}
