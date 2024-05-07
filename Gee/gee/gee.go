package gee

import (
	"log"
	"net/http"
	"path"
	"strings"
	"text/template"
)

type Handlerfunc func(*Context)

// Engine 类型定义了一个引擎结构体。
// 它包含一个路由器(router)、一个RouteGroup指针、以及一个存储所有路由分组的切片(groups)。
type Engine struct {
	router        *router            // 负责路径匹配和处理的路由器
	*RouteGroup                      // 基础路由分组，提供路由创建的快捷方法
	groups        []*RouteGroup      // 存储所有路由分组，用于管理路由的组织结构
	htmlTemplates *template.Template // 用于渲染HTML模板的模板引擎
	funcMap       template.FuncMap   // 用于模板渲染时的函数映射
}

// RouteGroup 类型定义了一个路由分组结构体。
// 路由分组允许为一组路由设置共同的前缀(prefix)和中间件(middleware)。
// 它还维护了一个指向其父分组(parent)的指针，以及指向所属引擎(engine)的指针，以便于路由解析和处理。
type RouteGroup struct {
	prefix     string        // 路由分组的共同前缀
	middleware []Handlerfunc // 路由分组的共同中间件函数列表
	parent     *RouteGroup   // 父路由分组，用于实现嵌套分组
	engine     *Engine       // 所属的引擎实例
}

// 创建一个引擎结构体
func New() *Engine {
	engine := &Engine{router: newRouter()}
	engine.RouteGroup = &RouteGroup{engine: engine}
	engine.groups = []*RouteGroup{engine.RouteGroup}
	return engine
}
func (group *RouteGroup) Use(middleware ...Handlerfunc) {
	group.middleware = append(group.middleware, middleware...)
}
func (engine *Engine) SetFuncMap(funcMap template.FuncMap) {
	engine.funcMap = funcMap
}

// LoadHTMLGlob函数用于根据指定的模式加载HTML模板。
//
// 参数:
// pattern - 一个字符串，用于指定要加载的模板的模式。
//
// 此函数没有返回值。
func (engine *Engine) LoadHTMLGlob(pattern string) {
	// 使用指定的模式加载所有的HTML模板，并将它们存储在engine的htmlTemplates字段中。
	engine.htmlTemplates = template.Must(template.New("").Funcs(engine.funcMap).ParseGlob(pattern)) //这个函数用于解析一个目录下的所有模板文件，并将它们加载到一个模板集中。
}

// Group 创建一个新的路由分组，该分组继承当前分组的前缀和引擎，
// 同时添加一个新的前缀。
// 这允许对路由进行更细致的分组管理。
//
// 参数:
//
//	prefix string - 新分组将添加的前缀。
//
// 返回值:
//
//	*RouteGroup - 新创建的路由分组的指针。
func (group *RouteGroup) Group(prefix string) *RouteGroup {
	engine := group.engine // 引用当前分组所属的引擎
	// 创建一个新的路由分组，继承当前分组的前缀并添加新的前缀，同时设置当前分组为父分组
	newRoute := &RouteGroup{
		prefix: group.prefix + prefix,
		parent: group,
		engine: engine,
	}
	// 将新创建的路由分组添加到引擎的分组列表中
	engine.groups = append(engine.groups, newRoute)
	return newRoute // 返回新创建的路由分组
}
func (group *RouteGroup) addRoute(method string, comp string, handler Handlerfunc) {
	pattern := group.prefix + comp
	log.Printf("Route %4s - %s", method, pattern)
	group.engine.router.addRoute(method, pattern, handler)
}

// ServeHTTP 是 Engine 类型的 HTTP 请求处理方法。
// 它负责根据请求的 URL 路径匹配对应的处理组，并将匹配到的中间件链设置为请求的处理者。
// 参数 w 用于向客户端发送响应；
// 参数 req 代表客户端的请求。
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var middlewares []Handlerfunc // 定义一个中间件切片，用于存储匹配到的中间件

	// 遍历所有处理组，检查请求的 URL 是否以处理组的前缀开头
	for _, group := range engine.groups {
		// 用于判断一个字符串（req.URL.Path 在本例中）
		// 是否以指定的前缀（group.prefix 在本例中）开始。
		if strings.HasPrefix(req.URL.Path, group.prefix) {
			// 如果匹配成功，则将该处理组的中间件追加到中间件切片中
			middlewares = append(middlewares, group.middleware...)
		}
		c := newContext(w, req)
		c.handler = middlewares
		c.engine = engine
		engine.router.handle(c)

	}

	// 创建一个新的上下文，用于处理当前请求
	c := newContext(w, req)
	// 设置上下文的处理者为匹配到的中间件链
	c.handler = middlewares
	// 使用路由器处理请求
	engine.router.handle(c)
}

// 定义GET方法
func (group *RouteGroup) GET(patten string, handler Handlerfunc) {
	group.addRoute("GET", patten, handler)
}

// 定义POST方法
func (group *RouteGroup) POST(patten string, handler Handlerfunc) {
	group.addRoute("POST", patten, handler)
}

// 启动服务器
func (engine *Engine) Run(addr string) (err error) {
	return http.ListenAndServe(addr, engine)
}

// createStaticHandler 创建一个处理静态文件请求的Handler。
// relativePath 相对于路由组前缀的相对路径。
// fs 是实现http.FileSystem接口的文件系统。
// 返回一个Handlerfunc，用于处理静态文件请求。
func (group *RouteGroup) createStaticHandler(relativePath string, fs http.FileSystem) Handlerfunc {
	// 将路由组前缀和相对路径合并，得到服务静态文件的绝对路径。
	absolutePath := path.Join(group.prefix, relativePath)
	// 使用http.StripPrefix处理请求的路径，确保能够正确地从文件系统中获取文件。
	fileServer := http.StripPrefix(absolutePath, http.FileServer(fs))
	// 返回处理静态文件请求的函数。
	return func(c *Context) {
		// 从请求中获取文件路径参数。
		file := c.Param("filepath")
		// 尝试打开文件，如果失败则返回404状态码。
		if _, err := fs.Open(file); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		// 使用fileServer处理请求并响应。
		fileServer.ServeHTTP(c.Writer, c.Req)
	}
}

// Static 注册一个处理静态文件请求的路由。
// relativePath 是相对于应用根路径的静态资源路径。
// root 是静态资源在文件系统中的根目录。
func (group *RouteGroup) Static(relativePath string, root string) {
	// 创建处理静态文件请求的handler。
	handler := group.createStaticHandler(relativePath, http.Dir(root))
	// 构建匹配静态文件请求的URL模式。
	urlPattern := path.Join(relativePath, "/*filepath")
	// 使用GET方法注册处理静态文件请求的路由。
	group.GET(urlPattern, handler)
}
