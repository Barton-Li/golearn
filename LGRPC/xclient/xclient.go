package xclient

import (
	. "LGRPC"
	"context"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *Option
	mu      sync.Mutex
	clients map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *Option) *XClient {
	return &XClient{
		d:       d,
		mode:    mode,
		opt:     opt,
		clients: make(map[string]*Client),
	}
}
func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

// dial尝试与指定的RPC地址建立连接。如果该地址的连接已存在但不可用，则关闭并删除该连接，然后重新创建一个新的连接。
// 如果该地址的连接不存在，则直接创建一个新的连接。
// 参数：
//
//	rpcAddr string - RPC服务的地址。
//
// 返回值：
//
//	*Client - 成功时返回与RPC地址对应的客户端实例。
//	error - 如果建立连接过程中出现错误，则返回错误信息；否则返回nil。
func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	// 同步锁定，确保并发安全
	xc.mu.Lock()
	defer xc.mu.Unlock() // 确保函数退出时释放锁

	// 尝试从已建立的连接池中获取客户端
	client, ok := xc.clients[rpcAddr]
	// 如果连接存在但不可用，则关闭、删除并重置该连接
	if ok && !client.IsAvailable() {
		_ = client.Close()          // 关闭连接，忽略返回错误
		delete(xc.clients, rpcAddr) // 从池中删除该地址的连接
		client = nil                // 重置client为nil
	}

	// 如果该地址没有连接，则创建一个新的连接
	if client == nil {
		var err error
		client, err = XDial(rpcAddr, xc.opt) // 尝试建立新连接
		if err != nil {
			return nil, err // 如果建立连接失败，返回错误信息
		}
		xc.clients[rpcAddr] = client // 将新建立的连接加入到连接池中
	}

	return client, nil // 返回建立的连接和nil错误
}

// call是XClient的内部方法，用于执行RPC调用。
//
// rpcAddr: 表示RPC服务的地址。
// ctx: 传递上下文，用于控制调用的取消、超时等。
// ServiceMethod: 要调用的服务方法名称。
// args: 调用方法的参数。
// reply: 调用方法的返回结果。
// 返回值: 调用过程中可能出现的错误。
func (xc *XClient) call(rpcAddr string, ctx context.Context, ServiceMethod string, args interface{}, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err // 在建立连接时遇到错误，直接返回错误。
	}
	return client.Call(ctx, ServiceMethod, args, reply)
}

// Call是XClient的公开方法，用于对外提供RPC调用的接口。
//
// ctx: 传递上下文，用于控制调用的取消、超时等。
// ServiceMethod: 要调用的服务方法名称。
// args: 调用方法的参数。
// reply: 调用方法的返回结果。
// 返回值: 调用过程中可能出现的错误。
//
// 这个方法首先通过xc.d.Get(xc.mode)获取RPC服务的地址，然后调用内部方法call完成实际的调用过程。
func (xc *XClient) Call(ctx context.Context, ServiceMethod string, args interface{}, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err // 获取服务地址失败，直接返回错误。
	}
	return xc.call(rpcAddr, ctx, ServiceMethod, args, reply)
}

// Broadcast 广播调用所有服务地址的指定方法。
// ctx: 上下文，用于控制调用的取消和超时。
// ServiceMethod: 要调用的服务方法名称。
// args: 调用方法的参数。
// reply: 用于存储调用结果的结构体，可以为nil。
// 返回值: 执行过程中遇到的第一个错误，如果没有错误则为nil。
func (xc *XClient) Broadcast(ctx context.Context, ServiceMethod string, args interface{}, reply interface{}) error {
	// GetAllAndCall 对所有服务地址进行RPC调用，并收集结果。

	// 获取所有服务地址
	servers, err := xc.d.GetAll()
	if err != nil {
		return err // 获取服务地址失败，直接返回错误。
	}

	// 使用WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup
	// 使用Mutex保护共享资源
	var mu sync.Mutex
	// 存储遇到的第一个错误
	var e error
	// 判断是否已经有回复结果
	replyDone := reply == nil

	// 创建可取消的上下文
	// context.WithCancel返回一个派生的上下文，该上下文包含了一个取消函数。
	// 当调用该取消函数时，派生的上下文将被标记为取消，其任何仍处于等待状态的子操作都会被取消。
	// 参数：
	//   ctx - 基础上下文，通常是来自函数参数的context.Context。
	// 返回值：
	//   context.Context - 派生的上下文，可以在子操作中使用以检测取消信号。
	//   cancel - 取消函数，调用该函数会标记上下文为取消状态，通知任何相关操作终止。
	ctx,cancel:=context.WithCancel(ctx)

	// 遍历所有服务地址并并发调用
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			// 如果有reply，则创建其克隆用于存储调用结果
			var cloneReply interface{}
			if reply != nil {
				// 通过反射创建一个新的reply实例
				// 此代码段并没有作为函数或方法的一部分，因此不涉及参数和返回值说明。
				// 但是，这段代码的目的是通过反射机制，基于已有的reply对象类型，
				// 创建一个新的相同类型的对象实例。这个新实例会被赋值给cloneReply变量。
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}

			// 执行RPC调用
			err := xc.call(rpcAddr, ctx, ServiceMethod, args, cloneReply)
			mu.Lock()
			// 如果遇到错误且当前还没有记录错误，则更新错误信息并取消上下文
			if err != nil && e == nil {
				e = err
				cancel()
			}
			// 如果调用成功且还未记录回复，则更新回复并标记已完成
			if err == nil && !replyDone {
				// 将 cloneReply 的值赋给 reply。
				// 此操作首先通过 reflect.ValueOf 获取 reply 和 cloneReply 的值，
				// 然后通过 Elem() 方法取得它们的元素值（假设它们是指针），
				// 最后使用 Set() 方法将 cloneReply 的元素值赋给 reply 的元素值。
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}

	// 等待所有goroutine完成
	wg.Wait()
	// 返回遇到的第一个错误
	return e
}
