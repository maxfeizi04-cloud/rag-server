package parser

import (
	"archive/zip"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// DOCXParser Word 文档解析器
// DOCX 文件本质是一个 ZIP 压缩包，其中的 word/document.xml 包含文本内容
// 不需要依赖任何第三方库，只需要 Go 标准库的 archive/zip + regexp
type DOCXParser struct{}

func (p *DOCXParser) Parse(filePath string) (string, error) {
	// 以 ZIP 方式打开 DOCX 文件
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("DOCX 解析失败: %w", err)
	}
	defer reader.Close()

	// 在 ZIP 包中查找 word/document.xml
	var docXML *zip.File
	for _, f := range reader.File {
		if f.Name == "word/document.xml" {
			docXML = f
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("DOCX 解析: 未找到 document.xml")
	}

	// 读取 XML 内容
	rc, err := docXML.Open()
	if err != nil {
		return "", fmt.Errorf("DOCX 解析: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("DOCX 解析: %w", err)
	}

	// DOCX 的文本内容在 <w:t> 标签中
	// 例如: <w:r><w:t>这是文本</w:t></w:r>
	// 正则提取所有 <w:t> 标签内的文本
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllSubmatch(data, -1)

	var parts []string
	for _, m := range matches {
		if len(m) > 1 && len(m[1]) > 0 {
			parts = append(parts, string(m[1]))
		}
	}

	// 在文本中标记段落边界（<w:p> 标签代表段落）
	text := string(data)
	text = regexp.MustCompile(`<w:p[ >]`).ReplaceAllString(text, "\n<w:p")

	result := strings.Join(parts, "")
	result = strings.TrimSpace(result)

	if result == "" {
		return "", fmt.Errorf("DOCX 解析: 未提取到文本")
	}
	return result, nil
}
