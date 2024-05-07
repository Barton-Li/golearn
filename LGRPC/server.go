package LGRPC

import (
	"LGRPC/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

// MagicNumber 是一个常量，用于标识某种特定的协议或数据格式的开始。
const (
	MagicNumber      = 0x3bef5c
	connected        = "200 Connected to Go RPC"
	defaultRPCPath   = "/lgrpc_"
	defaultDebugPath = "/debug/lgrpc"
)

// ServerHTTP 是Server类型的一个方法，用于处理HTTP请求。
// 该方法主要实现对CONNECT方法的特殊处理，通过Hijack接口接管连接，然后自定义处理逻辑。
// 参数:
//
//	w http.ResponseWriter - 用于向客户端发送响应的接口。
//	req *http.Request - 表示客户端发起的请求。
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 检查请求方法是否为CONNECT，如果不是，则返回405 Method Not Allowed。
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 Must CONNECT\n")
		return
	}

	// 尝试通过Hijacker接口 hijack 连接，如果失败则记录日志并返回。
	// 这个接口允许一个 HTTP 处理程序接管与客户端的连接，
	// 即在正常的请求-响应生命周期之外控制连接。
	// 这对于需要直接与底层 TCP 连接交互的场景非常有用，
	// 比如实现 WebSocket 协议、长轮询、或者其他自定义的双向通信协议。
	// 		首先，它通过类型断言 w.(http.Hijacker)
	//  确保 ResponseWriter 实现了 Hijacker 接口。
	// 然后，调用 Hijack() 方法获取到 TCP 连接 (net.Conn) 和用于读写的缓冲 (*bufio.ReadWriter)，
	// 以及一个可能的错误。
	// 如果 Hijack 成功，错误 (err) 应该为 nil，
	// 可以直接使用 conn 和读写器来处理这个连接，而不经过标准的 HTTP 协议栈。
	// 需要注意的是，HTTP/2 不支持 http.Hijacker 接口，
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc server: hijack error:", err)
		return
	}

	// 向连接发送HTTP 1.0的connected响应，然后通过ServeConn方法处理连接。
	_, _ = io.WriteString(conn, "HTTP/1.0"+connected+"\n\n")
	s.ServeConn(conn)
}
// HandleHTTP 设置HTTP处理程序。
// 该函数不接受参数，也不返回值。
// 它主要负责配置HTTP服务器以处理RPC请求和调试请求。
func (s *Server) HandleHTTP() {
    // 注册默认的RPC路径处理程序
    http.Handle(defaultRPCPath, s)
    // 注册默认的调试路径处理程序
    http.Handle(defaultDebugPath, debugHTTP{s})
    // 记录HTTP服务的监听路径
    log.Println("rpc server: serving http on", defaultRPCPath, "and", defaultDebugPath)
}
func HandleHTTP() {
	DefaultServer.HandleHTTP()
}

// Option 结构体用于配置某些功能或参数。
// 其中包括：
// MagicNumber - 一个整数，用于标识数据的开始或验证数据的正确性。
// CodecType - 指定使用的编解码类型，来自 codec 包的 Type 类型。
type Option struct {
	MagicNumber    int
	CodecType      codec.Type
	ConnectTimeout time.Duration
	HandleTimeout  time.Duration
}

// DefaultOption 定义了默认的选项。
// 这是一个全局变量，用于提供一个默认配置的实例，
// 主要包括MagicNumber和CodecType两个字段。
var DefaultOption = &Option{

	MagicNumber:    MagicNumber,      // MagicNumber 用于标识数据文件的版本或类型。
	CodecType:      codec.GOBType,    // CodecType 指定了序列化和反序列化时使用的编解码器类型。
	ConnectTimeout: time.Second * 10, // ConnectTimeout 定义了等待客户端连接的超时时间。
}

type Server struct {
	serviceMap sync.Map
}

// request 结构体用于表示一个请求
type request struct {
	h            *codec.Header // codec.Header 指向请求的头部信息
	argv, replyv reflect.Value // argv 和 replyv 分别表示请求的参数值和回复的值，使用 reflect.Value 存储，允许动态类型
	mtype        *methodType   // 指向请求的方法类型
	svc          *service      // 指向请求的服务实例
}

// 注册将新的服务添加到服务器，前提是该服务尚未注册。
// 它接受一个实现了服务方法的接收者对象，并以其唯一的名称将其注册。
// 如果服务已注册，将返回错误表示服务已被定义。
//
// 参数：
// - rcvr: 实现服务方法的接收者对象。用于生成服务实例。
//
// 返回值：
// - error: 如果服务已注册，返回表示服务已定义的错误对象；否则返回nil。
func (s *Server) Register(rcvr interface{}) error {
	// 从接收者创建新的服务实例。
	ser := newService(rcvr)

	// 尝试从映射中加载或存储服务，并检查是否已存储，
	// 以防止重复的服务注册。
	if _, dup := s.serviceMap.LoadOrStore(ser.name, s); dup {
		return errors.New("rpc server: service already defined" + ser.name)
	}
	return nil
}
func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

// findService在给定的服务映射中查找指定的服务和方法。
// 它首先通过"."来分割服务名和方法名，然后在服务映射中查找服务，
// 如果找到服务，则在该服务的方法映射中查找方法。
// 如果服务或方法不存在，将返回相应的错误。
//
// 参数:
//
//	serviceMethod - 要查找的服务和方法的字符串表示，格式为"服务名.方法名"
//
// 返回值:
//
//	*service - 找到的服务实例，如果未找到则为nil
//	*methodType - 找到的方法类型，如果未找到则为nil
//	error - 查找过程中遇到的错误，如果未遇到错误则为nil
func (s *Server) findService(serviceMehtod string) (svc *service, mtype *methodType, err error) {
	// 使用"."查找服务名和方法名之间的分割点
	// 其作用是在目标字符串中查找指定子串最后一次出现的位置索引。
	// 如果未找到子串，则返回 -1。
	dot := strings.LastIndex(serviceMehtod, ".")
	if dot < 0 {
		// 如果找不到"."，则说明请求的格式错误
		err = errors.New("rpc server:service/method request ill-formed: " + serviceMehtod)
		return
	}
	// 根据"."分割服务名和方法名
	serviceName, methodName := serviceMehtod[:dot], serviceMehtod[dot+1:]
	// 从服务映射中加载指定的服务名
	svci, ok := s.serviceMap.Load(serviceName)
	if !ok {
		// 如果服务不存在，则返回错误
		err = errors.New("rpc server:can't find service " + serviceName)
		return
	}
	// 将加载的服务转换为service类型
	svc = svci.(*service)
	// 从服务中查找指定的方法
	mtype = svc.method[methodName]
	if mtype == nil {
		// 如果方法不存在，则返回错误
		err = errors.New("rpc server:can't find method " + methodName)
	}
	return
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

// Accept 方法在给定的监听器上等待新的连接。
// 一旦有新的连接进来，它会通过 ServeConn 方法为每个连接提供服务。
//
// 参数:
// lis - 用于监听网络连接的net.Listener对象。
//
// 无返回值，但会在接受连接出错时记录日志并退出函数。
func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept() // 尝试接受一个新的网络连接。
		if err != nil {
			log.Println("rpc server:accept error:", err) // 当接受连接出错时，记录错误信息。
			return
		}
		go s.ServeConn(conn) // 在一个新的goroutine中为接受到的连接提供服务。
	}
}

// Accept 函数使用默认服务器实例接受新的连接。
// lis: 表示监听网络连接的Listener对象。
func Accept(lis net.Listener) { DefaultServer.Accept(lis) }

// ServeConn 函数处理单个连接的RPC服务。
// conn: 表示客户端连接的io.ReadWriteCloser对象。
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	// 确保连接在函数返回前被关闭
	defer func() {
		_ = conn.Close()
	}()
	var opt Option
	// 尝试从连接解码选项信息
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server:options error", err)
		return
	}
	// 校验接收到的选项中的魔法数字是否正确
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server:invaild magic number %x", opt.MagicNumber)
		return
	}
	// 根据选项中的编解码类型获取相应的编解码函数
	f := codec.NewCodeFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server:invaild codec type %s", opt.CodecType)
		return
	}
	// 使用获取到的编解码函数处理连接
	s.serverCodec(f(conn), &opt)
}

// invalidRequest 为一个空结构体，用于表示无效的请求。
// 在Go语言中，结构体为空时，不会占用任何存储空间。
var invalidRequest = struct{}{}

// readRequestHeader 读取请求头
// param cc codec.Codec 用于读取Header的codec对象
// return *codec.Header 返回读取到的Header，如果读取失败则返回nil
// return error 返回读取过程中的错误，如果没有错误则返回nil
func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil { // 尝试从cc读取Header到h

		// 说明遇到了除正常结束和预期外的EOF之外的其他错误。在这种情况下，
		// 程序选择记录一个错误日志，表明在读取 RPC 请求头时发生了问题。
		// 这样做的目的是区分常见的、可以接受的读取结束（io.EOF）
		// 与那些可能导致数据不完整或解析失败的异常情况（io.ErrUnexpectedEOF），
		// 以便于调试和故障排查。
		if err != io.EOF && err != io.ErrUnexpectedEOF { // 如果错误不是EOF或ErrUnexpectedEOF，则记录错误日志
			log.Println("rpc server:read header error:", err)
		}

		return nil, err // 返回nil和错误
	}
	return &h, nil // 成功读取Header后返回
}

// readRequest 从给定的codec中读取请求。
//
// 参数:
//
//	cc codec.Codec - 用于读取请求的codec。
//
// 返回值:
//
//	*request - 读取到的请求对象。
//	error - 读取过程中遇到的任何错误。
func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	// 从codec中读取请求头
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err // 如果读取请求头时出现错误，返回nil和错误信息
	}
	req := &request{h: h}

	// 查找服务和方法类型
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return req, err // 如果查找服务时出现错误，返回请求对象和错误信息
	}

	// 初始化请求的参数和回复值
	req.argv = req.mtype.newArgs()
	req.replyv = req.mtype.newReplyv()

	// 根据参数类型，调整argv为合适的接口类型
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	// 从codec中读取请求体
	if err := cc.ReadBody(argvi); err != nil {
		return req, err // 如果读取请求体时出现错误，返回请求对象和错误信息
	}

	return req, nil // 返回成功读取的请求对象和nil错误
}

// sendResponse 通过指定的codec向客户端发送响应。
//
// 参数:
// cc - 用于编码和解码消息的codec。
// h - 响应头，包含响应的元数据。
// body - 要发送的响应体，具体类型取决于调用方和codec的支持。
// sending - 一个互斥锁，用于控制发送操作的并发访问。
//
// 该函数不返回任何值。
func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	// 加锁以确保发送操作的原子性，防止并发冲突。
	sending.Lock()
	defer sending.Unlock() // 确保在函数返回时释放锁。

	// 尝试写入响应，如果遇到错误则记录日志。
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

// handleRequest 处理服务请求.
// cc: 用于编码和解码的编解码器.
// req: 包含请求信息的结构体.
// sending: 用于控制发送响应的互斥锁.
// wg: 用于等待所有请求处理完成的等待组.
// timeout: 请求的超时时间.
func (s *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	// 确保请求处理完成后通知等待组
	defer wg.Done()

	// 创建两个通道，以协调请求的调用和发送
	called := make(chan struct{})
	sent := make(chan struct{})

	// 并发处理请求
	go func() {
		// 调用服务方法
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{} // 通知请求已调用

		if err != nil {
			// 如果有错误，设置错误信息并发送错误响应
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{} // 通知响应已发送
			return
		}

		// 无错误，发送正常响应
		s.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{} // 通知响应已发送
	}()

	// 处理超时逻辑
	if timeout == 0 {
		<-called // 等待请求调用完成
		<-sent   // 等待响应发送完成
		return
	}

	select {
	case <-time.After(timeout): // 超时未完成，发送超时错误响应
		req.h.Error = fmt.Sprintf("rpc server: call timeout %s", timeout)
		s.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called: // 请求调用完成
		<-sent // 等待响应发送完成
	}
}

// serverCodec是一个在Server实例上设置codec并处理请求的函数。
// 它不断地读取请求，处理它们，并发送响应，直到读取请求时发生不可恢复的错误。
// 参数:
// - cc: codec.Codec，用于编码和解码消息的编解码器实例。
// - opt: *Option，包含服务器操作的选项，例如处理超时时间。
func (s *Server) serverCodec(cc codec.Codec, opt *Option) {
	// 创建一个互斥锁，用于控制对发送响应的访问。
	sending := new(sync.Mutex)
	// 创建一个等待组，用于等待所有处理请求的goroutine完成。
	wg := new(sync.WaitGroup)

	for {
		// 尝试从连接读取一个请求。
		req, err := s.readRequest(cc)
		if err != nil {
			// 如果发生错误，检查是否是可忽略的错误（如连接关闭）。
			if req == nil {
				break // 如果是可忽略的错误，则退出循环。
			}
			// 设置请求的错误信息，并发送一个错误响应。
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			continue // 继续读取下一个请求。
		}

		// 为处理请求增加等待组计数。
		wg.Add(1)
		// 开启一个goroutine来处理请求。
		go s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}

	// 等待所有goroutine处理完请求。
	wg.Wait()
	// 尝试关闭编解码器连接。
	_ = cc.Close()
}
