package parser

import "os"

// TextParser 纯文本解析器
// 适用于 .txt 和 .md 文件
// 直接读取文件全部内容作为文本返回
type TextParser struct{}

func (p *TextParser) Parse(filePath string) (string, error) {
	data, err := os.ReadFile(filePath) // Go 1.16+ 的 os.ReadFile 等价于 ioutil.ReadFile
	if err != nil {
		return "", err
	}
	return string(data), nil
}
