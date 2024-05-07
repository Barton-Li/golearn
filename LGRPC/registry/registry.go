package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type LGRegistry struct {
	timeout time.Duration
	mu      sync.Mutex
	servers map[string]*ServerItem
}
type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultTimeout = time.Minute * 5
	defaultPath    = "/lgrpc_/registry"
)

func New(timeout time.Duration) *LGRegistry {
	return &LGRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultLGRegistry = New(defaultTimeout)

// putServer 将指定地址的服务器信息记录到注册表中。
// 如果该地址的服务器信息不存在，则新建一个服务器项并记录其开始时间；
// 如果已存在，则更新其开始时间为当前时间。
// 参数：
//
//	addr string - 服务器的地址。
//
// 返回值：
//
//	无。
func (r *LGRegistry) putServer(addr string) {
	r.mu.Lock()         // 加锁，确保并发安全
	defer r.mu.Unlock() // 确保函数退出时释放锁

	// 尝试获取给定地址的服务器项
	s := r.servers[addr]
	// 如果服务器项不存在，则创建一个新的服务器项并记录其开始时间
	if s == nil {
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		// 如果服务器项已存在，仅更新其开始时间为当前时间
		s.start = time.Now()
	}
}

// aliveServer 返回当前注册表中所有存活的服务器地址列表。
// 该方法首先锁定注册表以确保并发安全，然后遍历服务器列表。
// 对于每个服务器，如果其启动时间加上指定的超时时间在当前时间之后，
// 则认为该服务器是存活的，并将其地址添加到结果列表中。
// 否则，将该服务器从注册表中删除。
// 最后，对结果列表进行排序，并返回该列表。
// 返回值:
// []string: 存活服务器的地址列表，按字母顺序排序。
func (r *LGRegistry) aliveServer() []string {
	r.mu.Lock()         // 锁定注册表以确保并发安全
	defer r.mu.Unlock() // 确保函数退出时解锁

	var alive []string // 存储存活服务器地址的列表
	for addr, s := range r.servers {
		// 判断服务器是否存活
		// 检查服务器的活跃截止时间是否在当前时间之后。如果是，则表示服务器仍然活跃。
		// 如果超时时间为0（意味着没有超时限制）或者服务器的活跃截止时间在现在之后，
		// 那么服务器被认为是存活的
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr) // 从注册表中删除已超时的服务器
		}
	}
	sort.Strings(alive) // 对存活服务器地址列表进行排序
	return alive
}
// ServeHTTP 是LGRegistry类型的方法，用于处理HTTP请求。
// 它根据请求的方法类型(GET或POST)来执行相应的逻辑。
// 参数:
//   w http.ResponseWriter - 用于向客户端发送HTTP响应的接口。
//   req *http.Request - 表示客户端发起的HTTP请求的结构体指针。
func (r *LGRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// 当请求方法为GET时，将存活的服务器地址以逗号分隔并设置到响应头的"X-LGRPC-Servers"字段。
		w.Header().Set("X-LGRPC-Servers",strings.Join(r.aliveServer(),","))
	case "POST":
		// 当请求方法为POST时，从请求头的"X-LGRPC-Server"字段获取服务器地址。
		addr:=req.Header.Get("X-LGRPC-Server")
		if addr==""{
			// 如果没有获取到服务器地址，返回500内部服务器错误。
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 将获取到的服务器地址添加到注册表。
		r.putServer(addr)
	default:
		// 对于其他请求方法，返回405方法不允许的状态码。
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
// HandleHTTP 将 HTTP 请求处理逻辑绑定到指定的路径上。
// 参数:
//   registryPath string - 注册表的HTTP访问路径。
// 该函数没有返回值。
func (r *LGRegistry) HandleHTTP(registryPath string) {
    // 将指定路径的HTTP请求交给r处理
    http.Handle(registryPath, r)
    // 记录注册表的HTTP访问路径日志
    log.Println("rpc-registry path:", registryPath)
}

// HandleHTTP 是 LGRegistry 类的默认实例的HTTP处理函数。
// 该函数将默认路径的HTTP请求处理逻辑绑定到DefaultLGRegistry实例上。
// 该函数没有参数和返回值。
func HandleHTTP() {
    // 绑定DefaultLGRegistry到默认路径上处理HTTP请求
    DefaultLGRegistry.HandleHTTP(defaultPath)
}
// sendHeartbeat 向指定的注册中心发送心跳信号。
// registry: 注册中心的URL地址。
// addr: 当前RPC服务器的地址。
// 返回值: 发送过程中遇到的任何错误。
func sendHeartbeat(registry, addr string) error {
    // 记录发送心跳的日志信息
    log.Println("send heartbeat to", registry, addr)
    
    // 创建一个HTTP客户端
    httpClient := &http.Client{}
    
    // 构建向注册中心发送的POST请求，设置特定的请求头
    req, _ := http.NewRequest("POST", registry, nil)
    req.Header.Set("X-LGRPC-Server", addr)
    
    // 执行HTTP请求，如果出现错误，则记录日志并返回错误
    if _, err := httpClient.Do(req); err != nil {
        log.Println("rpc sever: heart beat err:", err)
        return err
    }
    
    // 如果请求成功执行，则返回nil表示无错误发生
    return nil
}
// Heartbeat函数用于定期向指定地址发送心跳信号。
// registry: 注册中心的地址。
// addr: 需要发送心跳的目标地址。
// duration: 心跳发送的间隔时长。如果未指定或为0，则使用默认超时时间减去1分钟作为间隔。
func Heartbeat(registry, addr string, duration time.Duration) {
	// 若未指定间隔时长，则使用默认值
	if duration == 0 {
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	// 发送初始心跳
	err = sendHeartbeat(registry, addr)
	// 启动一个goroutine定时发送心跳
	go func() {
		//NewTicker函数来创建一个定时器，并使用<-t.C来阻塞等待定时器触发
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C // 等待定时器触发
			err = sendHeartbeat(registry, addr) // 发送心跳
		}
	}()
}