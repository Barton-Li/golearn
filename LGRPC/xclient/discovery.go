package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

// Refresh() 从注册中心更新服务列表
// Update(servers []string) 手动更新服务列表
// Get(mode SelectMode) 根据负载均衡策略，选择一个服务实例
// GetAll() 返回所有的服务实例
type SelectMode int

// 定义选择模式的常量
const (
	// RandomSelect 表示随机选择模式
	RandomSelect SelectMode = iota
	// RoundRobinSelect 表示轮询选择模式
	RoundRobinSelect
)

// Discovery 接口定义了服务发现的功能
type Discovery interface {
	// Refresh 用于刷新服务列表
	// 返回值: 如果刷新成功返回nil，否则返回错误信息
	Refresh() error
	// Update 用于更新服务列表
	// 参数: servers []string - 需要更新的服务地址列表
	Update(servers []string) error
	// Get 用于根据选择模式获取一个服务地址
	// 参数: mode SelectMode - 选择模式
	// 返回值: 选择的服务地址和可能的错误信息
	Get(mode SelectMode) (string, error)
	// GetAll 用于获取所有服务地址
	// 返回值: 所有服务地址的列表和可能的错误信息
	GetAll() ([]string, error)
}

type MultiServersDiscovery struct {
	r       *rand.Rand
	mu      sync.RWMutex
	servers []string
	index   int
}

// NewMultiServersDiscovery 创建一个 MultiServersDiscovery 实例。
//
// servers: 提供一个字符串切片，包含了服务器的地址。
// 返回值: 返回初始化好的 *MultiServersDiscovery 实例指针。
func NewMultiServersDiscovery(servers []string) *MultiServersDiscovery {
	// 初始化 MultiServersDiscovery 实例，并使用随机数生成器来选择一个起始索引
	d := &MultiServersDiscovery{
		servers: servers,
		// 初始化一个随机数生成器
		// 使用当前时间的纳秒级Unix时间戳作为随机数生成器的种子，以确保每次运行程序时生成的随机数序列不同。
		r: rand.New(rand.NewSource(int64(time.Now().UnixNano()))),
	}
	// 设置一个随机的起始索引，范围在 0 到 math.MaxInt32-1 之间
	d.index = d.r.Intn(math.MaxInt32 - 1)
	return d
}

// _ Discovery 是一个类型断言，表示 *MultiServersDiscovery 实现了 Discovery 接口。
// 这种方式通常用于类型检查。
var _ Discovery = (*MultiServersDiscovery)(nil)


// Refresh 方法用于刷新服务器列表。
// 返回值 error: 返回错误信息，当前实现中总是返回 nil。
func (d *MultiServersDiscovery) Refresh() error {
	return nil
}

// Update 方法用于更新服务器列表。
// 参数 servers []string: 要更新的服务器地址列表。
// 返回值 error: 返回错误信息，当前实现中总是返回 nil。
func (d *MultiServersDiscovery) Update(servers []string) error {
	d.mu.Lock() // 加锁以保护并发访问
	defer d.mu.Unlock() // 确保在函数返回前释放锁
	d.servers = servers // 更新服务器列表
	return nil
}
// Get 从多个服务器中选择一个进行返回。
// 
// 参数:
//   mode SelectMode - 选择模式，可以是随机选择或轮询选择。
// 
// 返回值:
//   string - 选中的服务器地址。
//   error - 如果没有可用服务器或选择模式无效，则返回错误。
func (d *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock() // 加锁以确保并发安全
	defer d.mu.Unlock() // 确保函数退出时解锁

	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no server available") // 如果没有可用服务器，返回错误
	}

	switch mode {
	case RandomSelect:
		// 随机选择一个服务器
		return d.servers[d.r.Intn(n)], nil
	case RoundRobinSelect:
		// 轮询选择一个服务器，并更新选择索引
		s := d.servers[d.index%n]
		d.index = (d.index + 1) % n
		return s, nil
	default:
		// 如果选择模式无效，返回错误
		return "", errors.New("rpc discovery: invalid select mode")
	}
}
// GetAll 方法从 MultiServersDiscovery 实例中获取所有服务器的地址列表。
// 
// 返回值:
// []string: 一个包含所有服务器地址的字符串切片。
// error: 如果获取过程中发生错误，则返回错误对象；否则返回 nil。
func (d *MultiServersDiscovery) GetAll() ([]string, error) {
	// 同步读取操作，确保在读取过程中数据不会被其他写操作改变。
	d.mu.RLock()
	defer d.mu.RUnlock() // 确保在函数返回时释放读锁，避免死锁。

	// 拷贝服务器地址列表到新的切片，以避免外部修改影响到内部状态。
	servers := make([]string, len(d.servers))
	copy(servers, d.servers)

	return servers, nil
}
