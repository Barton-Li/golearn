package registry

import (
	"net/http"
	"sort"
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
func (r *LGRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set()
	case "POST":
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
