// Markdown 报告 → .docx（OOXML）导出。
// 不依赖第三方库：直接组装 WordprocessingML 包（document/styles/rels）。
// 支持标题、段落、粗体/行内代码、代码块、列表、表格、分隔线；中文由 Word 字体回退渲染。
package ops

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

const wns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type docxRun struct {
	text string
	bold bool
	code bool
}

// parseInline 解析 **粗体** 与 `行内代码`，其余按纯文本输出。
func parseInline(s string) []docxRun {
	var runs []docxRun
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			runs = append(runs, docxRun{text: plain.String()})
			plain.Reset()
		}
	}
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "**") {
			if end := strings.Index(s[i+2:], "**"); end >= 0 {
				flush()
				runs = append(runs, docxRun{text: s[i+2 : i+2+end], bold: true})
				i += 2 + end + 2
				continue
			}
		}
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				flush()
				runs = append(runs, docxRun{text: s[i+1 : i+1+end], code: true})
				i += 1 + end + 1
				continue
			}
		}
		plain.WriteByte(s[i])
		i++
	}
	flush()
	if len(runs) == 0 {
		runs = append(runs, docxRun{text: ""})
	}
	return runs
}

func docxRunsXML(runs []docxRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString("<w:r>")
		if r.bold || r.code {
			b.WriteString("<w:rPr>")
			if r.bold {
				b.WriteString("<w:b/>")
			}
			if r.code {
				b.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="18"/>`)
			}
			b.WriteString("</w:rPr>")
		}
		b.WriteString(`<w:t xml:space="preserve">` + xmlEscape(r.text) + "</w:t></w:r>")
	}
	return b.String()
}

func docxPara(runs []docxRun, style, indent string) string {
	var pPr strings.Builder
	if style != "" {
		pPr.WriteString(`<w:pStyle w:val="` + style + `"/>`)
	}
	if indent != "" {
		pPr.WriteString(`<w:ind w:left="` + indent + `"/>`)
	}
	s := "<w:p>"
	if pPr.Len() > 0 {
		s += "<w:pPr>" + pPr.String() + "</w:pPr>"
	}
	return s + docxRunsXML(runs) + "</w:p>"
}

func docxTable(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:color="999999"/><w:left w:val="single" w:sz="4" w:color="999999"/>` +
		`<w:bottom w:val="single" w:sz="4" w:color="999999"/><w:right w:val="single" w:sz="4" w:color="999999"/>` +
		`<w:insideH w:val="single" w:sz="4" w:color="999999"/><w:insideV w:val="single" w:sz="4" w:color="999999"/>` +
		`</w:tblBorders></w:tblPr>`)
	for ri, row := range rows {
		b.WriteString("<w:tr>")
		for _, cell := range row {
			runs := parseInline(strings.TrimSpace(cell))
			if ri == 0 {
				for i := range runs {
					runs[i].bold = true
				}
			}
			b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="0" w:type="auto"/></w:tcPr>` + docxPara(runs, "", "") + "</w:tc>")
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	// 表后补一个空段落，避免与下文粘连
	return b.String() + docxPara([]docxRun{{text: ""}}, "", "")
}

func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	return strings.Split(line, "|")
}

func isTableSep(line string) bool {
	for _, c := range splitTableRow(line) {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		for _, ch := range c {
			if ch != '-' && ch != ':' {
				return false
			}
		}
	}
	return strings.Contains(line, "-")
}

// markdownToDocxBody 将 Markdown 逐块转为 WordprocessingML body 片段。
func markdownToDocxBody(md string) string {
	var b strings.Builder
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	inCode := false
	var codeLines []string
	flushCode := func() {
		for _, cl := range codeLines {
			b.WriteString(docxPara([]docxRun{{text: cl, code: true}}, "Code", ""))
		}
		codeLines = nil
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
			}
			inCode = !inCode
			continue
		}
		if inCode {
			codeLines = append(codeLines, line)
			continue
		}
		if trimmed == "" {
			continue
		}
		// 表格：当前行含 | 且下一行是分隔行
		if strings.Contains(trimmed, "|") && i+1 < len(lines) && isTableSep(lines[i+1]) {
			rows := [][]string{splitTableRow(trimmed)}
			i += 2
			for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
				rows = append(rows, splitTableRow(lines[i]))
				i++
			}
			i--
			b.WriteString(docxTable(rows))
			continue
		}
		// 标题
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level <= 6 && level < len(trimmed) && trimmed[level] == ' ' {
				b.WriteString(docxPara(parseInline(strings.TrimSpace(trimmed[level:])), fmt.Sprintf("Heading%d", level), ""))
				continue
			}
		}
		// 分隔线
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			b.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="999999"/></w:pBdr></w:pPr></w:p>`)
			continue
		}
		// 无序列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			b.WriteString(docxPara(parseInline("• "+trimmed[2:]), "", "420"))
			continue
		}
		// 有序列表
		if dot := strings.Index(trimmed, ". "); dot > 0 && dot <= 3 {
			num := trimmed[:dot]
			isNum := true
			for _, ch := range num {
				if ch < '0' || ch > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				b.WriteString(docxPara(parseInline(num+". "+trimmed[dot+2:]), "", "420"))
				continue
			}
		}
		// 引用
		if strings.HasPrefix(trimmed, "> ") {
			b.WriteString(docxPara(parseInline(trimmed[2:]), "Quote", "420"))
			continue
		}
		b.WriteString(docxPara(parseInline(trimmed), "", ""))
	}
	if inCode {
		flushCode()
	}
	return b.String()
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`

const docxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docxDocRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

func docxStyles() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<w:styles ` + wns + `>`)
	b.WriteString(`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="宋体"/><w:sz w:val="21"/></w:rPr></w:rPrDefault></w:docDefaults>`)
	sizes := map[int]string{1: "36", 2: "30", 3: "26", 4: "24", 5: "22", 6: "21"}
	for lvl := 1; lvl <= 6; lvl++ {
		b.WriteString(fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:pPr><w:outlineLvl w:val="%d"/><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr><w:b/><w:sz w:val="%s"/></w:rPr></w:style>`, lvl, lvl, lvl-1, sizes[lvl]))
	}
	b.WriteString(`<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:pPr><w:shd w:val="clear" w:fill="F5F5F5"/></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="18"/></w:rPr></w:style>`)
	b.WriteString(`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:rPr><w:i/><w:color w:val="666666"/></w:rPr></w:style>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

// buildDocx 生成完整 .docx 文件字节。
func buildDocx(title, markdown string) ([]byte, error) {
	body := docxPara(parseInline(title), "Heading1", "") + markdownToDocxBody(markdown)
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:document ` + wns + `><w:body>` + body +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr>` +
		`</w:body></w:document>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	files := map[string]string{
		"[Content_Types].xml":        docxContentTypes,
		"_rels/.rels":                docxRels,
		"word/_rels/document.xml.rels": docxDocRels,
		"word/styles.xml":            docxStyles(),
		"word/document.xml":          document,
	}
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}