package codec

import "io"

// 抽象出 Codec 的构造函数，客户端和服务端可以通过 Codec 的 Type 得到构造函数，
// 从而创建 Codec 实例。
// 这部分代码和工厂模式类似，
// 与工厂模式不同的是，返回的是构造函数，而非实例。

type Header struct {
	ServiceMethod string //服务名和方法名，与go结构体和方法映射
	Seq           uint64 //序列号，用于标识请求和响应

	Error string //错误信息
}
// 通过定义这个接口，可以统一不同类型的编解码器的实现，
// 并通过这个接口来实现客户端和服务端的解耦。
type Codec interface {
	io.Closer                 // 用于关闭连接
	ReadHeader(*Header) error // ReadHeader 用于读取消息头。
	// 参数:
	//   *Header: 指向要填充的 Header 结构体的指针。
	// 返回值:
	//   error: 如果读取过程中发生错误，则返回非 nil 错误。

	ReadBody(interface{}) error // ReadBody 用于读取消息体。
	// 参数:
	//   interface{}: 用于接收读取的消息体的变量。可以根据实际需要断言为具体的类型。
	// 返回值:
	//   error: 如果读取过程中发生错误，则返回非 nil 错误。

	Write(*Header, interface{}) error // Write 用于写入数据。
	// 参数:
	//
	// 参数:
	// - header: 指向 Header 的指针，用于指定写入操作的头部信息。
	// - data: 任意类型的数据，将被写入。此参数的类型为 interface{}，允许接收任意类型的值。
	// 返回值:
	//   error: 如果写入过程中发生错误，则返回非 nil 错误。
}

// NewCodeFunc 用于创建新的 Codec。
// 参数:
//   io.ReadWriteCloser: 用于读写的连接。
// 返回值:
//   Codec: 新的 Codec。
type NewCodeFunc func(io.ReadWriteCloser) Codec

type Type string

// 定义了 JSON 和 GOB 两种编码方式的类型常量。
// GOB是Go语言内置的编码方式，全称为Go Binary。
// 它专门用于在Go程序中进行数据的编码和解码。
// JSON是一种常见的基于文本的编码方式，
// 它可以将结构化的数据转换为JSON格式的字符串。
const (
	JSONType Type = "application/json"
	GOBType  Type = "application/gob"
)

//
// NewCodeFuncMap 是一个映射，它将 Type 类型作为键，
// NewCodeFunc 作为值，用于存储根据不同类型创建新代码的函数。
// 这样做的目的是为了能够根据需要动态地创建不同类型的新代码实例。
var NewCodeFuncMap map[Type]NewCodeFunc

//
func init() {
	NewCodeFuncMap = make(map[Type]NewCodeFunc)
	// NewCodeFuncMap[JSONType] = NewJSONCodec
	NewCodeFuncMap[GOBType] = NewGobCodec
}
