package LGRPC

import (
	"LGRPC/codec"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// clientResult 结构体用于存储客户端操作的结果
// 其中包含：
// client *Client - 成功创建的客户端实例指针
// err error - 在创建客户端过程中遇到的错误
type clientResult struct {
	client *Client
	err    error
}

// newClientFunc 是一个函数类型，用于定义创建新客户端的函数
// 其接受一个网络连接（conn net.Conn）和一个选项配置（opt *Option）
// 并返回一个客户端实例指针（client *Client）和一个错误（err error）
// 函数的主要任务是根据传入的网络连接和选项配置来创建一个新的客户端实例
type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

// dialTimeout 函数尝试建立一个具有超时控制的网络连接，并使用给定的 newClientFunc
// 函数初始化一个客户端。
//
// 参数:
//
//	f newClientFunc - 一个用于根据连接和选项创建客户端实例的函数。
//	network string - 指定要使用的网络类型（如 "tcp" 或 "udp"）。
//	address string - 指定要连接的远程地址。
//	opts ...*Option - 一个或多个选项参数，用于自定义连接行为。
//
// 返回值:
//
//	client *Client - 成功时返回初始化好的客户端实例。
//	err error - 连接或客户端初始化过程中出现的错误。
func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	// 解析提供的选项参数。
	opt, err := parseOptins(opts...)
	if err != nil {
		return nil, err
	}

	// 建立网络连接并设置超时。
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}

	// 确保在函数返回前关闭连接。
	// 此代码块为匿名函数，作为defer语句的一部分，确保在函数返回前执行。
	// 主要功能是检查err变量是否为nil，如果不为nil，则尝试关闭conn。
	defer func() {
		// 检查err是否非nil，如果是，尝试关闭连接，并忽略关闭操作的错误。
		if err != nil {
			_ = conn.Close()
		}
	}()
	// 使用通道来异步处理客户端的初始化。
	ch := make(chan clientResult)
	// 此匿名函数执行特定的任务：使用`conn`和`opt`调用`f`函数，然后将结果发送到`ch`通道。
	// 它不返回任何值，但通过通道通信来传递结果。
	go func() {
		// 尝试根据`conn`和`opt`配置客户端。
		client, err := f(conn, opt)

		// 将客户端实例和可能出现的错误作为结果发送到通道。
		ch <- clientResult{client: client, err: err}
	}()
	// 如果未设置连接超时，则等待客户端初始化完成。
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}

	// 如果设置了连接超时，则在指定时间内等待客户端初始化或超时。
	// 等待连接成功或超时。
	//
	// 参数:
	//   opt: 包含连接超时时间的选项。
	//   ch: 用于接收连接结果的通道。
	//
	// 返回值:
	//   client: 成功连接时返回的客户端实例。
	//   err: 连接失败时返回的错误信息。
	select {
	// 用于创建一个定时器，在指定的时间后向返回的通道发送一个时间值。
	// 它的作用是在指定的时间间隔之后向你提供一个通知，可以用于定时执行某个操作。
	// 常见的应用场景包括定时执行任务、超时控制等，
	// 它使得程序能够方便地在指定的时间间隔后进行操作，
	// 而无需通过复杂的定时器管理来实现。
	case <-time.After(opt.ConnectTimeout):
		// 连接超时，返回错误
		return nil, errors.New("connect timeout")
	case result := <-ch:
		// 接收到连接结果，返回客户端和错误信息
		return result.client, result.err
	}

}
func Dial(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}

// Call 结构体表示一个远程过程调用
type Call struct {
	Seq           uint64      // Seq 是调用的序列号
	ServiceMethod string      // ServiceMethod 是被调用的服务方法名
	Args          interface{} // Args 是调用方法时传递的参数
	Reply         interface{} // Reply 是用于接收调用结果的响应
	Error         error       // Error 用于记录调用过程中发生的错误
	Done          chan *Call  // Done 是一个通道，用于在调用完成后通知调用方
}

// done 方法用于标记调用完成，并通过 Done 通道通知调用方
func (call *Call) done() {
	call.Done <- call
}

// Client 结构体代表了一个客户端。
type Client struct {
	cc       codec.Codec      // cc 用于消息的编解码。
	opt      *Option          // opt 存储客户端的配置选项。
	sending  sync.Mutex       // sending 用于控制发送操作的并发访问。
	header   codec.Header     // header 存储消息的头部信息。
	mu       sync.Mutex       // mu 用于控制对客户端状态的并发访问。
	seq      uint64           // seq 用于标识每个请求的序列号。
	pending  map[uint64]*Call // pending 存储尚未完成的调用请求。
	closing  bool             // closing 表示客户端是否正在关闭。
	shutdown bool             // shutdown 表示客户端是否已经关闭。
}

// 类型断言，判断client是否实现了io.Closer接口
var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("lgrpc: the connect is shut down")

// Close 关闭客户端连接。
// 返回值表示关闭操作是否成功。如果客户端已经在关闭过程中，则返回 ErrShutdown 错误。
func (client *Client) Close() error {
	client.mu.Lock()         // 加锁，确保并发安全
	defer client.mu.Unlock() // 确保函数退出时解锁
	if client.closing {
		return ErrShutdown // 如果已经在关闭中，则返回错误
	}
	client.closing = true    // 标记为关闭状态
	return client.cc.Close() // 关闭底层连接
}

// IsAvailable 检查客户端是否可用。
// 返回值表示客户端是否可用。客户端不可用的条件是：正在关闭中或者已经关闭。
func (client *Client) IsAvailable() bool {
	client.mu.Lock()         // 加锁
	defer client.mu.Unlock() // 解锁
	// 判断客户端是否可用
	return !client.closing && !client.shutdown
}

// registerCall 用于注册一个调用请求
// 参数:
//
//	client *Client: 客户端实例
//	call *Call: 要注册的调用请求
//
// 返回值:
//
//	uint64: 调用的序列号
//	error: 错误信息，如果注册成功则为nil
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()         // 加锁保护客户端状态
	defer client.mu.Unlock() // 确保在函数结束时解锁

	// 检查客户端是否正在关闭或已经关闭
	if client.closing || client.shutdown {
		return 0, ErrShutdown // 如果是，则返回错误
	}

	// 为调用请求设置序列号，并将其添加到待处理列表中
	call.Seq = client.seq
	client.pending[call.Seq] = call // 将调用添加到待处理列表

	client.seq++         // 序列号自增
	return call.Seq, nil // 返回调用的序列号
}

// removeCall 从客户端的待处理调用列表中移除指定序列号的调用，并返回该调用。
// 参数：
// seq - 调用的序列号。
// 返回值：
// 返回被移除的调用对象指针。
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()         // 加锁保护客户端状态，确保在操作过程中数据的一致性和安全性
	defer client.mu.Unlock() // 确保在函数执行完毕后释放锁，避免死锁

	call := client.pending[seq] // 从待处理调用列表中获取指定序列号的调用对象
	delete(client.pending, seq) // 移除待处理调用列表中指定序列号的调用对象
	return call
}

// terminateCalls 在服务端或客户端发生错误时被调用，用于设置统一的错误信息到所有进行中的调用。
// 参数:
//
//	err error - 需要设置给所有进行中调用的错误信息。
func (client *Client) terminateCalls(err error) {
	client.sending.Lock()         // 加锁以确保并发安全，避免在修改进行中调用列表时发生竞态条件
	defer client.sending.Unlock() // 确保函数退出时释放锁，防止死锁

	client.mu.Lock()         // 对Client实例加锁，确保在更新内部状态时的并发安全
	defer client.mu.Unlock() // 确保函数退出时释放锁，防止死锁

	client.shutdown = true // 标记客户端为关闭状态，防止进一步的调用添加

	// 遍历所有待处理的调用，设置它们的错误信息并标记为完成
	for _, call := range client.pending {
		call.Error = err
		call.done() // 通知调用方调用失败，让调用方能够据此进行相应的错误处理
	}
}

// receive 方法用于接收服务端响应。
// 此方法会在一个循环中不断尝试读取响应的头信息和正文，直到发生错误或读取完成所有响应。
// 当读取到响应头后，会根据序列号查找对应的调用请求，并根据响应头中的信息决定如何处理响应正文。
// 参数:
// - client *Client: 表示客户端实例，用于进行远程调用。
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		// 尝试读取响应头，如果失败则退出循环。
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		// 根据响应头的序列号，查找对应的调用请求。
		call := client.removeCall(h.Seq)
		switch {
		// 如果找不到对应的调用请求，读取并丢弃响应正文。
		case call == nil:
			err = client.cc.ReadBody(nil)
		// 如果响应头中包含错误信息，设置调用请求的错误信息，并读取并丢弃响应正文。
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		// 如果响应头正常，尝试读取响应正文，并将其设置到调用请求的回复对象中。
		default:
			err = client.cc.ReadBody(call.Reply)
			// 如果读取响应正文时发生错误，设置调用请求的错误信息。
			if err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	// 如果在读取过程中发生错误，终止所有进行中的调用，并设置统一的错误信息。
	client.terminateCalls(err)
}

// newClientCodec 创建一个新的客户端编码器。
//
// 参数:
// cc - 实现了codec.Codec接口的对象，用于消息的编解码。
// opt - 包含客户端配置选项的*Option指针。
//
// 返回值:
// 返回一个初始化好的*Client指针。
func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client := &Client{
		cc:      cc,                     // 指定的编解码器
		opt:     opt,                    // 客户端配置选项
		seq:     1,                      // 初始化序列号
		pending: make(map[uint64]*Call), // 初始化一个用于存储待处理请求的映射
	}
	go client.receive() // 开启一个goroutine用于接收消息
	return client
}

// NewClient 创建一个新的Client实例。
//
// conn: 网络连接，用于与服务端进行通信。
// opt: 选项，包含Codec类型等配置信息。
//
// 返回值 *Client: 创建的Client实例。
// 返回值 error: 如果创建过程中遇到错误，则返回错误信息。
func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	// 根据选项中的Codec类型，获取对应的编解码函数
	f := codec.NewCodeFuncMap[opt.CodecType]
	if f == nil {
		// 如果获取的编解码函数为nil，表示Codec类型无效，记录错误并返回
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client:codec error:", err)
		return nil, err
	}

	// 将选项信息编码并发送给服务端，如果编码过程中出现错误，记录错误信息，关闭连接并返回错误
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client:option error:", err)
		_ = conn.Close() // 关闭连接，_用于忽略返回值
		return nil, err
	}

	// 使用编解码函数和连接创建ClientCodec，并返回新的Client实例
	return newClientCodec(f(conn), opt), nil
}

// parseOptins 解析给定的选项参数，并返回一个配置好的 Option 实例。
// 如果提供的选项参数不满足预期，则返回默认选项或错误信息。
//
// 参数:
//
//	opts ...*Option - 可变数量的 Option 指针，代表用户指定的配置选项。
//
// 返回值:
//
//	*Option - 配置好的 Option 实例，基于输入参数进行解析和调整。
//	error - 如果解析过程中出现错误，则返回错误信息；否则返回 nil。
func parseOptins(opts ...*Option) (*Option, error) {
	// 检查输入参数，如果为空或第一个参数为 nil，则返回默认选项
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}

	// 确保只接受一个选项参数，否则报错
	if len(opts) != 1 {
		return nil, errors.New("only one option is allowed")
	}

	opt := opts[0]
	// 使用默认选项的 MagicNumber 值来补充未指定的 MagicNumber
	opt.MagicNumber = DefaultOption.MagicNumber
	// 如果未指定 CodecType，则使用默认的 CodecType
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

// 发送
// send 方法用于发送一个 RPC 调用请求。
// client: 表示客户端实例，用于发送请求和处理响应。
// call: 表示一个具体的 RPC 调用，包含调用的服务方法和参数等信息。
func (client *Client) send(call *Call) {
	// 使用互斥锁确保发送过程的线程安全
	client.sending.Lock()
	defer client.sending.Unlock()

	// 注册调用，获取唯一的序列号
	seq, err := client.registerCall(call)
	if err != nil {
		// 如果注册失败，设置错误信息并结束调用
		call.Error = err
		call.done()
		return
	}

	// 设置请求头的信息
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	// 尝试发送请求，如果出现错误，则处理错误并结束调用
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		// 从调用注册表中移除该调用
		call := client.removeCall(seq)
		if call != nil {
			// 如果成功移除，设置错误信息并结束调用
			call.Error = err
			call.done()
		}
	}
}

// Go 方法用于异步调用 RPC 服务的方法。
//
// 参数:
//
//	serviceMethod - 要调用的服务方法的名称。
//	args - 调用服务方法时传递的参数。
//	reply - 用于接收服务方法返回结果的变量。
//	done - 一个通道，用于通知调用者操作已完成。如果为 nil，将内部创建一个带缓冲的通道。
//
// 返回值:
//
//	返回一个 *Call 结构体指针，包含了此次调用的详细信息。
func (client *Client) Go(serviceMethood string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		// 如果 done 为 nil，内部创建一个带缓冲的通道，缓冲大小为 10。
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		// 如果 done 非 nil 但无缓冲，认为是不合法的配置，记录 panic 日志。
		log.Panic("rpc: done channel is unbuffered")
	}
	// 创建 Call 结构体实例，填充调用信息。
	call := &Call{

		ServiceMethod: serviceMethood,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call) // 将调用信息发送给客户端的发送队列。
	return call
}

// Call是一个用于发起RPC调用的方法。
// ctx: 传递上下文，用于控制调用的取消、超时等。
// 取消：当客户端不再需要等待服务端的响应时，可以通过调用 ctx.CancelFunc() 来取消请求，此时 ctx.Done() 通道会被关闭，Call 函数会立即返回一个表示取消的错误。
// 超时：可以使用 context.WithTimeout() 或 context.WithDeadline() 创建一个具有超时限制的新上下文。当超过指定的超时时，ctx.Done() 通道会被关闭，Call 函数同样会返回一个表示超时的错误。
// 依赖管理：ctx 还可以用于传递请求相关的附加信息，如请求ID、元数据等，方便在服务间跟踪和日志记录。
// serviceMethod: 要调用的服务方法名称。
// args: 调用参数。
// reply: 用于接收调用返回的结果。
// 返回值: 调用成功返回nil，否则返回错误信息。
func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	// 创建一个异步调用对象，并启动该调用
	// 调用远程服务的方法
	//
	// call: 用于执行服务方法的调用操作。
	// client: 引用的客户端实例，用于发起远程调用。
	// serviceMethod: 要调用的服务方法名称。
	// args: 调用服务方法时传递的参数。
	// reply: 用于接收服务方法返回的结果。
	// make(chan *Call, 1): 创建一个带缓冲的通道，用于接收调用的响应。
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		// 如果上下文被取消或超时，从调用列表中移除该调用，并返回相应的错误
		client.removeCall(call.Seq)
		return errors.New("rpc: " + ctx.Err().Error())

	case <-call.Done:
		// 等待调用完成，返回调用的结果或错误
		return call.Error
	}
}

// NewHTTPClient 创建一个新的HTTP客户端实例。
// conn: 提供网络连接，用于与服务器建立HTTP连接。
// opt: 包含客户端配置选项的指针。
// 返回值: 成功时返回一个初始化好的Client指针和nil错误；失败时返回nil和错误信息。
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
	// 向服务器发送CONNECT请求
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))

	// 读取服务器的响应
	resq, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resq.Status == connected {
		// 如果连接成功，创建并返回新的Client实例
		return NewClient(conn, opt)
	}
	if err == nil {
		// 如果连接失败，但没有错误返回，创建一个新的错误实例
		err = errors.New("unexpected HTTP response: " + resq.Status)
	}
	return nil, err
}
func DialHTTP(network, address string, opt ...*Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opt...)
}

// 简化调用，设置统一入口XDial
// XDial 是一个用于根据给定的RPC地址和选项创建客户端实例的函数。
// 
// 参数:
//   rpcAddr string - 格式为"protocol@address"的RPC地址，其中protocol指定了通信协议，address是服务的地址。
//   opt ...*Option - 一个或多个Option配置项，用于自定义客户端行为。
// 
// 返回值:
//   *Client - 成功时返回一个客户端实例。
//   error - 如果创建过程中遇到错误，则返回错误信息。
func XDial(rpcAddr string, opt ...*Option) (*Client, error) {
	// 根据"@"分割RPC地址，验证格式是否正确
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client err:wrong format of rpc address %s,expect protocol@address", rpcAddr)
	}

	// 解析协议和地址
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		// 如果协议是"http"，则使用DialHTTP函数进行连接
		return DialHTTP("tcp", addr, opt...)
	default:
		// 对于其他协议，使用Dial函数进行连接
		return Dial(protocol, addr, opt...)
	}
}
