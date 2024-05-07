package gee

import "strings"

// 接下来我们实现的动态路由具备以下两个功能。
// 参数匹配:。例如 /p/:lang/doc，可以匹配 /p/c/doc 和 /p/go/doc。
// 通配*。例如 /static/*filepath，可以匹配/static/fav.ico，
// 也可以匹配/static/js/jQuery.js，这种模式常用于静态服务器，
// 能够递归地匹配子路径。

// 实现前缀树路由
// node 结构体表示树形结构的一个节点
type node struct {
	pattern  string  // pattern 表示节点匹配的模式字符串
	part     string  // part 表示模式字符串中的一部分，即该节点代表的具体内容
	children []*node // children 是该节点的子节点们，构成树的下一层级
	isWild   bool    // isWild 表示该节点的模式字符串是否包含通配符，true 表示包含，false 表示不包含
}

// 第一个匹配成功的节点，用于插入
// matchChild 用于在节点的子节点中查找匹配的子节点。
// part: 需要匹配的字符串。
// 返回值: 如果找到匹配的子节点，则返回该子节点的指针；否则返回nil。
func (n *node) matchChild(part string) *node {
	for _, child := range n.children { // 遍历所有子节点
		if child.part == part || child.isWild { // 如果子节点的part与目标字符串匹配，或者子节点是通配符节点，则返回该子节点
			return child
		}
	}
	// 遍历所有子节点后仍未找到匹配的子节点，返回nil
	return nil
}

// matchChildren 用于匹配当前节点的所有子节点，
// 返回与指定部分匹配或为通配符的子节点列表。
// part: 需要匹配的部分字符串
// 返回值: 匹配到的子节点列表
func (n *node) matchChildren(part string) []*node {
	nodes := make([]*node, 0) // 初始化一个空的节点切片，用于存放匹配结果

	for _, child := range n.children { // 遍历所有子节点
		if child.part == part || child.isWild { // 判断子节点是否与指定部分匹配或为通配符
			nodes = append(nodes, child) // 将匹配到的子节点添加到结果切片中
		}
	}
	return nodes
}

// 插入
// insert函数用于将给定的字符串模式插入到树中。
// pattern: 需要插入的字符串模式。
// parts: 模式字符串分割后的各个部分。
// height: 当前处理的部分在模式字符串中的层级高度。
func (n *node) insert(pattern string, parts []string, height int) {
	if len(parts) == height { // 当已经遍历完所有parts，即到达树的叶子节点时，设置当前节点的pattern为给定模式
		n.pattern = pattern
		return
	}
	part := parts[height]       // 获取当前处理的part
	child := n.matchChild(part) // 尝试匹配当前part是否有对应的子节点
	if child == nil {           // 如果没有匹配到子节点，创建一个新的子节点
		child = &node{part: part,
			isWild: part[0] == ':' || part[0] == '*'}
		n.children = append(n.children, child) // 将新子节点添加到当前节点的子节点列表中
	}
	// 递归调用insert函数，继续在子节点中插入剩余的parts
	child.insert(pattern, parts, height+1)
}

// search 在树中查找与给定路径匹配的节点。
//
// 参数:
// parts - 要查找的路径，由多个部分组成。
// height - 当前处理的路径部分的高度，根节点的高度为0。
//
// 返回值:
// 如果找到匹配的节点，则返回该节点的指针；否则返回nil。
func (n *node) search(parts []string, height int) *node {
	// 如果已经处理完所有路径部分，或者当前节点是通配符节点
	if len(parts) == height || strings.HasPrefix(n.part, "*") {
		// 如果当前节点没有指定模式，则认为没有匹配，返回nil
		if n.pattern == "" {
			return nil
		}
		// 找到匹配的节点，返回该节点
		return n
	}
	// 提取当前要处理的路径部分
	part := parts[height]
	// 寻找当前节点下与当前路径部分匹配的子节点
	children := n.matchChildren(part)
	// 遍历所有匹配的子节点，递归查找
	for _, child := range children {
		result := child.search(parts, height+1)
		// 如果在子节点中找到了匹配的节点，则返回该节点
		if result != nil {
			return result
		}
	}
	// 如果所有子节点都没有匹配的节点，则返回nil
	return nil
}
