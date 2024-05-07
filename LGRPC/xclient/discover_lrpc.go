package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type GRegistryDiscovery struct {
	*MultiServersDiscovery
	registry   string
	timeout    time.Duration
	lastUpdate time.Time
}

const defaultUpdateTimeout = time.Second * 10

func NewGRegistryDiscovery(registry string, timeout time.Duration) *GRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	d := &GRegistryDiscovery{
		MultiServersDiscovery: NewMultiServersDiscovery(make([]string, 0)),
		registry:              registry,
		timeout:               timeout,
	}
	return d
}
func (d *GRegistryDiscovery) Update(server []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = server
	d.lastUpdate = time.Now()
	return nil
}

// Refresh 如果上次更新到现在的时间已经超过指定超时时间，则从注册表更新服务器列表。
// 使用互斥锁确保线程安全的操作。
// 如果向注册表发起的HTTP GET请求失败，返回错误；否则返回nil。
func (d *GRegistryDiscovery) Refresh() error {
	d.mu.Lock()         // 确保通过锁定结构来保证线程安全性。
	defer d.mu.Unlock() // 在函数退出时释放锁。

	// 检查最后一次更新是否在超时时间内，如果是，则不需要刷新。
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		return nil
	}

	// 记录刷新操作以及注册表URL的日志。
	log.Println("rpc registry: refresh servers", d.registry)

	// 向注册表发起HTTP GET请求以获取服务器列表。
	resp, err := http.Get(d.registry)
	if err != nil {
		// 如果GET请求失败，记录并返回错误。
		log.Println("rpc registry: get registry error", err)
		return err
	}

	// 从响应头中提取服务器地址。
	servers := strings.Split(resp.Header.Get("X-LRPC-Servers"), ",")
	d.servers = make([]string, 0, len(servers))

	// 过滤并追加非空服务器地址到列表中。
	for _, s := range servers {
		// 用于去除字符串的首尾空白字符
		// 通常在处理用户输入、文件读取、或者从外部系统获取数据时，
		// 字符串的首尾可能会带有空白字符（如空格、制表符、换行符等）。
		// 为了确保数据的准确性以及避免不必要的错误，
		// 经常需要对字符串进行首尾的空白字符去除操作
		if strings.TrimSpace(s) != "" {
			d.servers = append(d.servers, strings.TrimSpace(s))
		}
	}

	// 更新上次服务器列表刷新的时间戳。
	d.lastUpdate = time.Now()

	return nil
}

// Get 从注册表发现服务，根据指定的选择模式获取一个服务地址。
//
// 参数:
//
//	mode SelectMode - 选择模式，用于从多个服务实例中选择一个。
//
// 返回值:
//
//	string - 选中的服务地址。
//	error - 如果在刷新或获取服务地址时遇到错误，则返回错误。
func (d *GRegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil { // 刷新服务发现数据，确保获取到的地址是最新的
		return "", err
	}
	return d.MultiServersDiscovery.Get(mode) // 从多服务器发现对象中获取指定模式的服务地址
}

// GetAll 获取注册表中所有服务地址。
//
// 返回值:
//
//	[]string - 所有服务地址的切片。
//	error - 如果在刷新或获取服务地址列表时遇到错误，则返回错误。
func (d *GRegistryDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil { // 刷新服务发现数据，确保获取到的地址列表是最新的
		return nil, err
	}
	return d.MultiServersDiscovery.GetAll() // 从多服务器发现对象中获取所有服务地址
}
