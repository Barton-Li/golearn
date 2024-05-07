package gee

import (
	"net/http"
	"strings"
)

type router struct {
	roots   map[string]*node
	handler map[string]Handlerfunc
}

func newRouter() *router {
	return &router{
		roots:   make(map[string]*node),
		handler: make(map[string]Handlerfunc),
	}
}
// handle 是一个处理HTTP请求的方法。
// 它根据请求的方法和路径来查找对应的路由，并执行相应的处理函数。
// 如果找到了匹配的路由，则执行对应的处理函数；如果没有找到，则返回404页面。
//
// 参数:
// - r *router: 是路由对象，用于存储和查找路由信息。
// - c *Context: 是上下文对象，包含了当前HTTP请求的方法、路径以及参数等信息。
func (r *router) handle(c *Context) {
	// 根据请求方法和路径获取匹配的路由和参数。
	n, params := r.getRoute(c.Method, c.Path)
	if n != nil {
		// 如果找到了匹配的路由，构建键并从路由处理器映射中获取对应的处理函数。
		key := c.Method + "-" + n.pattern
		c.Params = params // 将匹配到的参数设置到上下文对象中。
		c.handler = append(c.handler, r.handler[key]) // 将处理函数添加到上下文对象的处理器链中。
	} else {
		// 如果没有找到匹配的路由，添加一个返回404状态码的处理函数。
		c.handler = append(c.handler, func(c *Context) {
			c.String(http.StatusNotFound, "404 NOT FOUND: %s\n", c.Path)
		})
	}
	// 调用下一个处理函数，继续处理请求。
	c.Next()
}

// parsePattern 解析给定的模式字符串，将其分割为有意义的部分
// 参数：
// pattern - 待解析的模式字符串，预期以斜杠("/")分隔各个部分。
//
// 返回值：
// 返回一个字符串切片，包含解析后的模式部分。忽略空字符串部分。
// 如果模式中包含以星号("*")开头的部分，该部分之后的内容将被忽
func parsePattern(pattern string) []string {
	vs := strings.Split(pattern, "/") // 使用斜杠分割模式字符
	parts := make([]string, 0)        // 初始化一个空字符串切片，用于存储解析后的部分
	for _, item := range vs {
		if item != "" {
			parts = append(parts, item)
			if item[0] == '*' { // 如果当前部分以星号开头，停止解析
				break
			}
		}
	}
	return parts
}

// addRoute 向路由器添加一个新路由。
// method: HTTP方法，如GET、POST等。
// pattern: 路径模式，用于匹配请求的URL路径。
// handler: 与该路由匹配时执行的处理函数。
func (r *router) addRoute(method string, pattern string, handler Handlerfunc) {
	// 解析路径模式为更易处理的格式。
	parts := parsePattern(pattern)
	// 生成唯一的键，用于存储路由信息。
	key := method + "-" + pattern
	// 检查是否存在根节点，若不存在则创建。
	_, ok := r.roots[method]
	if ok {
		r.roots[method] = &node{}
	}
	// 将路由模式插入到树结构中，以便快速匹配。
	r.roots[method].insert(pattern, parts, 0)
	// 将处理函数与路由键对应起来，以便后续调用。
	r.handler[key] = handler
}

// getRoute根据HTTP方法和路径查找匹配的路由节点及其参数。
//
// 参数:
// method - HTTP方法（如GET、POST等）。
// path - 请求的路径。
//
// 返回值:
// *node - 匹配到的路由节点，如果未找到则为nil。
// map[string]string - 路径参数的映射，如果路径中包含参数，则在此返回，如果未找到路由则为nil。
func (r *router) getRoute(method string, path string) (*node, map[string]string) {
	// 解析路径模式，将其分割成部分。
	searchParts := parsePattern(path)
	params := make(map[string]string)
	// 尝试从method对应的根节点中查找路由。
	root, ok := r.roots[method]
	if !ok {
		return nil, nil
	}
	// 使用解析的路径部分搜索路由树。
	node := root.search(searchParts, 0)

	if node != nil {
		parts := parsePattern(node.pattern) // 解析匹配到的节点的模式，用于进一步提取参数。
		for index, part := range parts {
			if part[0] == ':' {
				// 如果模式部分以冒号开头，表示这是一个命名参数，将其映射到对应的路径部分。
				params[part[1:]] = searchParts[index]
			}
			if part[0] == '*' && len(part) > 1 {
				// 如果模式部分以星号开头，表示这是一个通配符参数，将其映射到剩余的所有路径部分。
				params[part[1:]] = strings.Join(searchParts[index:], "/")
				break
			}
		}
		return node, params
	}
	return nil, nil
}

