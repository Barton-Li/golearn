package gee

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type H map[string]interface{}

// Context 是一个结构体，用于封装HTTP请求处理过程中的上下文信息。
type Context struct {
	Writer     http.ResponseWriter // Writer 用于向客户端发送响应
	Req        *http.Request       // Req 表示客户端发起的HTTP请求
	engine     *Engine             // engine 是一个Engine类型的指针，用于存储当前应用的Engine实例
	Path       string              // Path 表示请求的路径
	Method     string              // Method 表示请求的方法
	Params     map[string]string   // Params 包含URL中的参数部分，键值对形式
	StatusCode int                 // StatusCode 用于记录将要发送给客户端的HTTP状态码
	handler    []Handlerfunc       // handler 是一个Handlerfunc类型的切片，用于存储待处理的处理函数
	index      int                 // index 表示当前处理函数的索引，用于迭代执行处理函数
}

func newContext(w http.ResponseWriter, req *http.Request) *Context {
	return &Context{
		Writer: w,
		Req:    req,
		Path:   req.URL.Path,
		Method: req.Method,
		index:  -1,
	}
}

// Next 方法用于执行下一个处理程序。
// 在Context中，它通过递增索引来遍历并执行所有的处理程序。
func (c *Context) Next() {
	c.index++           // 递增当前处理程序索引
	s := len(c.handler) // 获取处理程序总数
	for ; c.index < s; c.index++ {
		c.handler[c.index](c) // 执行当前索引对应的处理程序
	}
}

// Param 通过键获取路径参数的值。
// 参数：
//
//	key: 要获取参数值的键。
//
// 返回值：
//
//	string: 如果找到键，则返回对应的参数值；否则返回空字符串。
func (c *Context) Param(key string) string {
	value, ok := c.Params[key] // 尝试从路径参数中获取键对应的值
	if ok {
		return value // 如果键存在，则返回其值
	}
	return "" // 如果键不存在，则返回空字符串
}

// PostForm 获取POST表单中指定键的值。
// 参数：
//
//	key: 要获取表单值的键。
//
// 返回值：
//
//	string: 如果找到键，则返回对应的表单值；否则返回空字符串。
func (c *Context) PostForm(key string) string {
	return c.Req.FormValue(key) // 直接从请求的表单中获取指定键的值
}

//Query 从请求的URL查询参数中获取指定键(key)对应的值。
//
// 参数:
//   key: 需要查询的参数键名。
//
// 返回值:
//   返回键名对应的参数值，如果不存在则返回空字符串。
// 	c.Req：这是一个指向 *http.Request 结构体的指针，
// 表示当前上下文（Context 类型）中的 HTTP 请求。
// http.Request 结构体包含了请求的所有相关信息，
// 如请求方法、URL、头部、主体等。

// .URL：访问 http.Request 的 URL 字段，
// 该字段类型为 *url.URL。它表示请求的统一资源定位符（URL），
// 包含了协议、主机、路径、查询参数等信息。

// .Query()：调用 url.URL 的 Query() 方法，
// 返回一个类型为 url.Values 的值。
// url.Values 是一个映射类型（类似 map[string][]string），
// 用于存储 URL 查询参数。键（key）是查询参数名，
// 值（value）是一个字符串切片，因为一个参数名可能对应多个值。

// .Get(key)：调用 url.Values 的 Get(key string) string 方法，传入参数 key。
// 此方法在查询参数集合中查找与 key 相匹配的项，
// 并返回第一个对应的值（字符串）。如果找不到匹配的参数，
// 将返回空字符串。
//
//	c.Req.URL.Query()：访问 http.Request 的 URL 字段，
//
// 并调用 url.URL 的 Query() 方法，返回一个 url.Values 类型的值。
// url.Values 是一个映射类型（类似 map[string][]string），
// 用于存储 URL 查询参数。键（key）是查询参数名，
// 值（value）是一个字符串切片，因为一个参数名可能对应多个值。
func (c *Context) Query(key string) string {

	return c.Req.URL.Query().Get(key)
}

// Status 设置响应的状态码，并通过响应写入器写入该状态码。
//
// 参数:
//   code int - 要设置的状态码。
//
// 该方法不返回任何值。
// c.Writer：这是一个指向 http.ResponseWriter 接口的指针，
// 通常在处理 HTTP 请求的上下文中使用。
// http.ResponseWriter 接口定义了向客户端发送 HTTP 响应所需的方法，
// 包括写入响应头、响应体以及设置状态码等。

// .WriteHeader(code)：调用 http.ResponseWriter 接口的 WriteHeader(statusCode int) 方法。
// 该方法接受一个整数参数 statusCode，即要设置的 HTTP 状态码。

// code：在此表达式中，code 是传递给 WriteHeader() 方法的参数，代表要设置的 HTTP 状态码。HTTP 状态码是一个三位数字，用于指示 HTTP 响应的状态。常见的状态码包括：
// 2xx：成功类别，如 200（OK）、201（Created）等。
// 3xx：重定向类别，如 301（Moved Permanently）、302（Found）等。
// 4xx：客户端错误类别，如 400（Bad Request）、404（Not Found）等。
// 5xx：服务器错误类别，如 500（Internal Server Error）、503（Service Unavailable）等。
func (c *Context) Status(code int) {
	c.StatusCode = code        // 设置上下文中的状态码
	c.Writer.WriteHeader(code) // 使用响应写入器写入状态码
}

func (c *Context) SetHeader(key string, value string) {
	c.Writer.Header().Set(key, value)
}

// Fail用于在处理中间件时提前结束并返回错误信息。
// code: HTTP状态码，用于表示请求处理的结果状态。
// err: 错误信息，将以JSON格式返回给客户端。
func (c *Context) Fail(code int, err string) {
	// 设置当前处理进度索引为handler数组的长度，以表示处理结束
	c.index = len(c.handler)
	// 使用指定的状态码和错误信息生成JSON响应
	c.JSON(code, H{"msg": err})
}
func (c *Context) String(code int, format string, values ...interface{}) {
	// 其中，键为"Content-Type"，值为"text/plain"。
	// 这个操作通常用于指定响应的内容类型。
	c.SetHeader("Content-Type", "text/plain")
	c.Status(code)
	// 使用指定格式和参数，将格式化后的字符串写入响应体中
	//
	// 参数：
	//   format: 格式字符串，用于格式化输出的内容
	//   values: 一个或多个值，用于替换格式字符串中的占位符
	//
	// 返回值：
	//   无
	//
	// 注意：该代码片段假设 `c.Writer` 已经通过某个上下文被正确初始化，它是用于向客户端发送响应的接口。
	c.Writer.Write([]byte(fmt.Sprintf(format, values...)))
}

// Content-Type类型是HTTP头部字段，用于指示资源的媒体类型（MIME类型）。
// 这个字段在响应中提供了客户端实际内容的类型，而在请求中（如POST或PUT），
// 客户端告诉服务器实际发送的数据类型。
// 头的值可以包含媒体类型和字符集（charset），
// 以及在某些情况下，如请求中，还可能包含边界（boundary）参数。
// text/html：用于HTML文档。
// application/json：用于JSON数据。
// application/xml：用于XML数据。
// image/png：用于PNG图像。
// image/jpeg：用于JPEG图像。
// application/javascript：用于JavaScript文件。
// text/css：用于CSS样式表。
// application/x-www-form-urlencoded：用于表单数据，通常在POST请求中使用。
// multipart/form-data：用于表单数据，包含文件上传。
// text/plain：用于纯文本数据。

// JSON响应方法，用于将对象作为JSON格式返回给客户端。
//
// 参数:
// code - HTTP状态码，用于响应客户端请求的状态。
// obj - 要转换为JSON格式并返回给客户端的对象。
//
// 无返回值。
func (c *Context) JSON(code int, obj interface{}) {
	c.SetHeader("Content-type", "application/json")
	c.Status(code)
	// 创建JSON编码器，用于将对象编码为JSON格式
	encoder := json.NewEncoder(c.Writer)
	// 将对象编码为JSON并写入响应体
	err := encoder.Encode(obj)
	if err != nil {
		http.Error(c.Writer, err.Error(), 500)
	}
}
func (c *Context) Data(code int, data []byte) {
	c.Status(code)
	c.Writer.Write(data)
}

func (c *Context) HTML(code int, name string, data interface{}) {
	c.SetHeader("Content-Type", "text/html")
	c.Status(code)
	if err := c.engine.htmlTemplates.ExecuteTemplate(c.Writer, name, data); err != nil {
		c.Fail(500, err.Error())
	}
}
