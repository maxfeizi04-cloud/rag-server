package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFParser PDF 文档解析器
// 使用 ledongthuc/pdf 纯 Go 库提取文本
// 注意：该库对复杂 PDF（扫描版、含大量图片、加密）可能提取不到文本
// 企业场景后续可考虑改用命令行 pdftotext 或 OCR 方案
type PDFParser struct{}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) Parse(filePath string) (string, error) {
	// 打开 PDF 文件，返回文件句柄和 reader
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("PDF 解析失败: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer

	// 获取总页数
	numPage := r.NumPage()

	// 逐页提取文本
	for i := 1; i <= numPage; i++ {
		page := r.Page(i)
		// V.IsNull() 检查页面是否存在内容
		if page.V.IsNull() {
			continue
		}
		// GetPlainText 提取页面的纯文本（不带格式）
		text, err := page.GetPlainText(nil)
		if err != nil {
			// 单页解析失败不中断，跳过继续
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}

	result := strings.TrimSpace(buf.String())
	if result == "" {
		return "", fmt.Errorf("PDF 解析: 未提取到文本（可能是扫描版 PDF）")
	}
	return result, nil
}
