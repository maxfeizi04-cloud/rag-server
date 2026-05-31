package parser

import "fmt"

// Parser 文档解析器接口
// 所有文件格式的解析器都必须实现这个接口
// 输入文件路径，输出提取的纯文本
type Parser interface {
	Parse(filePath string) (string, error)
}

// registry 解析器注册表
// key: 文件类型（pdf, docx, md, txt）
// value: 对应的解析器实例
// 使用包级变量而非 sync.Map，因为注册在 main.go 启动时一次性完成，无并发写入
var registry = map[string]Parser{
	"txt": &TextParser{}, // .txt 和 .md 共用文本解析器
	"md":  &TextParser{},
}

// Register 注册一个新的解析器
// 在 main.go 中调用，将自定义解析器加入注册表
// 例如: parser.Register("pdf", parser.NewPDFParser())
func Register(fileType string, p Parser) {
	registry[fileType] = p
}

// Parse 根据文件类型选择合适的解析器并提取文本
// fileType: 文件扩展名（不含点），如 "pdf", "docx"
// filePath: 文件在磁盘上的完整路径
func Parse(fileType, filePath string) (string, error) {
	p, ok := registry[fileType]
	if !ok {
		return "", fmt.Errorf("不支持的文件类型: %s", fileType)
	}
	return p.Parse(filePath)
}
