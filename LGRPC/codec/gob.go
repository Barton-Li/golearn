package codec

import (
	"bufio"
	"encoding/gob"
	"io"
)

// 定义GobCodec结构体
type GobCodec struct {
	// io.ReadWriteCloser接口是一个组合接口，
	// 它包含了io.Reader、io.Writer和io.Closer接口的全部方法，
	// 同时还有一个Close()方法。这个接口通常被用来表示一个可读可写的数据流，
	// 并且可以被关闭。

	// 以下是io.ReadWriteCloser接口的定义：

	// type ReadWriteCloser interface {
	//     Reader
	//     Writer
	//     Closer
	// }
	// io.Reader接口用于读取数据。
	// io.Writer接口用于写入数据。
	// io.Closer接口用于关闭数据流。
	conn io.ReadWriteCloser
	// bufio.Writer： bufio.Writer是Go语言标准库中的类型，
	// 它实现了io.Writer接口，同时提供了缓冲功能，
	// 用于提高写入的性能。通过创建bufio.Writer，
	// 可以将效率低下的单个字节写入转换为更高效的大块数据写入。
	buf *bufio.Writer
	// 它用于从输入流中进行GOB数据的解码，
	// 并将解码后的数据加载到Go语言的数据结构中。
	dec *gob.Decoder
	// 它用于将Go语言的数据结构编码为GOB数据，并写入输出流中。
	enc *gob.Encoder
}

// 将(*GobCodec)(nil)转换为Codec类型，并进行类型断言。
// 如果GobCodec类型实现了Codec接口，
// 这个类型断言会成功；如果没有实现，则会在编译时产生错误。
var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}
// 实现Codec接口的ReadHeader，ReadBody，Write，Close方法。
// ReadHeader 从连接中读取并反序列化一个Header对象。
// 参数:
//   header *Header：指向要反序列化数据的目标Header对象的指针。
// 返回值:
//   error：如果读取或反序列化过程中发生错误，则返回错误信息；否则返回nil。
func (c *GobCodec) ReadHeader(header *Header) error {
	return c.dec.Decode(header)
}

// ReadBody 从连接中读取并反序列化一个body对象。
// 参数:
//   body interface{}：指向要反序列化数据的目标对象的空接口。
// 返回值:
//   error：如果读取或反序列化过程中发生错误，则返回错误信息；否则返回nil。
func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

// Write 将给定的Header和Body对象进行序列化并写入连接。
// 参数:
//   header *Header：要序列化并写入连接的Header对象。
//   body interface{}：要序列化并写入连接的Body对象。
// 返回值:
//   error：目前总是返回nil，表示操作成功。未来的实现可能会处理写入错误。
func (c *GobCodec) Write(header *Header, body interface{}) error {
	return nil
}

// Close 关闭编解码器的连接。
// 返回值:
//   error：如果关闭连接时发生错误，则返回错误信息；否则返回nil。
func (c *GobCodec) Close() error {
	return c.conn.Close()
}