package LGRPC

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// methodType 结构体用于描述一个方法包括其参数和返回值的信息以及被调用的次数。
// 在需要动态分析或调用方法时，可以使用 reflect 包中的方法获取到
// methodType 结构中存储的信息，从而对方法进行分析和调用。
type methodType struct {
	method    reflect.Method // method 字段存储了方法的详细信息，包括方法名和方法函数体。
	ArgType   reflect.Type   // ArgType 字段定义了方法的参数类型。
	ReplyType reflect.Type   // ReplyType 字段定义了方法的返回值类型。
	numCalls  uint64         // numCalls 字段记录了该方法被调用的次数。
}

func (m *methodType) NewCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

// newArgs 创建并返回一个用于存储方法参数的reflect.Value实例。
// 该函数检查方法参数的类型，如果是指针类型，则创建一个指向该类型的指针的新实例；
// 如果不是指针类型，则创建该类型的值的新实例。
// 返回值 args 代表了新创建的参数实例。
func (m *methodType) newArgs() reflect.Value {
	var args reflect.Value
	// 根据参数类型是指针还是非指针，创建相应的实例
	if m.ArgType.Kind() == reflect.Ptr {
		// 如果参数类型是指针，则创建一个指向该类型元素的新指针
		args = reflect.New(m.ArgType.Elem())
	} else {
		// 如果参数类型不是指针，则直接创建该类型的实例
		args = reflect.New(m.ArgType).Elem()
	}
	return args
}

// newReplyv 为 *methodType 类型的方法，创建并返回一个对应 ReplyType 的新实例的 reflect.Value。
// 该方法首先通过 reflect.New 创建一个 ReplyType 的指针类型的实例，
// 然后根据 ReplyType 的元素类型（Elem()）进行初始化，特别是针对 map 和 slice 类型进行特殊处理。
// 参数:
//
//	m *methodType: 指向 methodType 类型的方法接收者。
//
// 返回值:
//
//	reflect.Value: 代表新创建的 ReplyType 实例的 reflect.Value。
func (m *methodType) newReplyv() reflect.Value {
	replyv := reflect.New(m.ReplyType.Elem()) // 创建一个 ReplyType 的元素类型的指针实例。

	switch m.ReplyType.Elem().Kind() { // 根据元素类型的种类进行不同处理。
	case reflect.Map:
		// 如果是 map 类型，初始化一个空的 map。
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		// 如果是 slice 类型，初始化一个长度和容量都为 0 的 slice。
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv // 返回初始化后的实例的 reflect.Value。
}

// service 结构体用于描述一个服务
type service struct {
	name   string                 // 名称：服务的名称
	typ    reflect.Type           // 类型：服务的类型
	rcvr   reflect.Value          // 接收者：服务的方法接收者
	method map[string]*methodType // 方法：服务的方法映射，键为方法名
}

// isExportedOrBuiltinType 函数判断给定的类型是否为导出类型或内置类型。
//
// 参数:
// t - reflect.Type类型，代表待检查的Go类型。
//
// 返回值:
// 返回一个布尔值，当类型为导出类型或内置类型时，返回true；否则返回false。
func isExportedOrBuiltinType(t reflect.Type) bool {
	// 检查类型是否为导出类型或其包路径为空（表示为内置类型）
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// registerMethods注册service中的方法到method映射中。
// 它遍历service类型的所有方法，选择符合特定签名的方法进行注册。
// 被注册的方法必须有三个输入参数和一个输出参数，且输出参数类型必须是error的子类型。
// 输入的第二个参数是请求类型，第三个参数是响应类型。只有当请求和响应类型是导出的或内置类型时，方法才会被注册。
func (service *service) registerMethods() {
	service.method = make(map[string]*methodType)
	for i := 0; i < service.typ.NumMethod(); i++ {
		method := service.typ.Method(i) // 获取当前方法
		mtype := method.Type

		// 检查方法类型是否符合特定的输入和输出参数数量要求。
		// 如果方法类型不符合要求，则继续进行下一次迭代。
		// 参数:
		//   mtype - 方法的类型信息。
		// 返回值:
		//   无。
		if mtype.NumIn() != 3 || mtype.NumOut() != 1 {
			continue
		}
		// 检查方法的返回值是否为error类型
		// 遍历所有方法，对每个方法进行检查。
		// 如果方法的第一个返回值类型不是error，则继续检查下一个方法。
		if mtype.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		// 分别获取请求类型和响应类型
		argType, replyType := mtype.In(1), mtype.In(2)
		// 检查请求和响应类型是否为导出的或内置类型
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		// 注册方法
		service.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server:register method %s,%s\n", service.name, method.Name) // 记录注册方法的日志
	}
}

// newService创建并初始化一个新的service实例。
// rcvr：要提供服务的结构体实例，必须是导出的类型。
// 返回值：初始化后的service指针。
func newService(rcvr interface{}) *service {
	s := new(service)                               // 创建service实例
	s.rcvr = reflect.ValueOf(rcvr)                  // 将传入的实例转换为reflect.Value类型
	s.typ = reflect.TypeOf(rcvr)                    // 将传入的实例类型转换为reflect.Type类型
	s.name = reflect.Indirect(s.rcvr).Type().Name() // 获取实例类型的名称

	// 检查类型名称是否为导出的（首字母大写）
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not exported", s.name) // 如果不是导出的，则记录错误并退出程序
	}
	s.registerMethods() // 注册服务的方法
	return s
}

// call 是一个用于执行服务方法的函数。
// 它通过反射来调用指定的方法，传递参数，并接收返回值。
// 参数:
// - m: 指向methodType的指针，包含了方法的信息。
// - args: 反射值，表示调用方法时的参数。
// - replyv: 反射值，用于接收方法的返回值。
// 返回值:
// - error: 如果调用过程中出现错误，则返回错误对象；否则返回nil。
func (service *service) call(m *methodType, args, replyv reflect.Value) error {
	// 增加方法调用次数的计数器
	atomic.AddUint64(&m.numCalls, 1)
	// 获取方法的函数对象
	f := m.method.Func
	// 调用方法，传入接收者、参数和返回值反射值
	returnValues := f.Call([]reflect.Value{service.rcvr, args, replyv})
	// 检查方法返回的错误
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
