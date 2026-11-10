package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

type UploadResultItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FileName    string `json:"file_name"`
	ObjectCount int    `json:"object_count"`
}

type UploadResponseData struct {
	SuccessCount int                `json:"successCount"`
	FailCount    int                `json:"failCount"`
	Modules      []UploadResultItem `json:"modules"`
	Errors       []string           `json:"errors"`
}

// ProcessUploadedMibFiles 处理由前端上传的 MIB / Excel / ZIP 文件
func ProcessUploadedMibFiles(files []*multipart.FileHeader) (*UploadResponseData, error) {
	resp := &UploadResponseData{
		Modules: []UploadResultItem{},
		Errors:  []string{},
	}

	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			resp.FailCount++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: 打开失败 - %v", fh.Filename, err))
			continue
		}

		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			resp.FailCount++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: 读取失败 - %v", fh.Filename, err))
			continue
		}

		ext := strings.ToLower(filepath.Ext(fh.Filename))
		switch ext {
		case ".xlsx", ".xls":
			if err := parseAndSaveExcel(fh.Filename, content, resp); err != nil {
				resp.FailCount++
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: Excel 解析失败 - %v", fh.Filename, err))
			}
		case ".zip":
			if err := parseAndSaveZip(fh.Filename, content, resp); err != nil {
				resp.FailCount++
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: ZIP 解压解析失败 - %v", fh.Filename, err))
			}
		default:
			// 默认为 MIB / 纯文本 / MY 文件
			if err := parseAndSaveTextMib(fh.Filename, ensureUtf8(string(content)), resp); err != nil {
				resp.FailCount++
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: MIB 文件解析失败 - %v", fh.Filename, err))
			}
		}
	}

	// 解析保存后派生最新的命名指标
	_ = SeedSnmpMetricDefinitions()

	return resp, nil
}

// parseAndSaveExcel 解析 Excel 文件 (.xlsx)
func parseAndSaveExcel(filename string, content []byte, resp *UploadResponseData) error {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("无法读取 Excel 内容: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return fmt.Errorf("Excel 中没有工作表")
	}

	moduleName := strings.TrimSuffix(filename, filepath.Ext(filename))
	moduleID := uuid.NewString()
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	var allObjects []repos.MibObject

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) == 0 {
			continue
		}

		// 列索引初始化
		oidIdx, nameIdx, typeIdx, descIdx, accessIdx := -1, -1, -1, -1, -1

		// 检查首行/前6行表头
		headerRowIdx := -1
		for rIdx, row := range rows {
			if rIdx > 5 {
				break
			}
			for cIdx, colVal := range row {
				val := strings.ToLower(strings.TrimSpace(ensureUtf8(colVal)))

				if val == "oid" || val == "节点oid" || val == "对象oid" || val == "node oid" || val == "标识符" || val == "索引" {
					oidIdx = cIdx
				} else if strings.Contains(val, "oid") && oidIdx == -1 && !strings.Contains(val, "parent") {
					oidIdx = cIdx
				}

				if val == "node name" || val == "name" || val == "节点名称" || val == "对象名称" || val == "指标名称" || val == "英文名称" || val == "symbol" || val == "指标名" {
					nameIdx = cIdx
				} else if (strings.Contains(val, "name") || strings.Contains(val, "名称")) && nameIdx == -1 {
					nameIdx = cIdx
				}

				if val == "node type" || val == "type" || val == "syntax" || val == "节点类型" || val == "数据类型" || val == "语法" || val == "类型" {
					typeIdx = cIdx
				} else if (strings.Contains(val, "type") || strings.Contains(val, "syntax") || strings.Contains(val, "类型") || strings.Contains(val, "语法")) && typeIdx == -1 {
					typeIdx = cIdx
				}

				if val == "description" || val == "desc" || val == "描述" || val == "节点描述" || val == "说明" || val == "指标说明" {
					descIdx = cIdx
				} else if (strings.Contains(val, "desc") || strings.Contains(val, "描述") || strings.Contains(val, "说明")) && descIdx == -1 {
					descIdx = cIdx
				}

				if val == "access" || val == "读写属性" || val == "访问权限" || val == "权限" {
					accessIdx = cIdx
				} else if (strings.Contains(val, "access") || strings.Contains(val, "权限")) && accessIdx == -1 {
					accessIdx = cIdx
				}
			}
			if oidIdx != -1 && nameIdx != -1 {
				headerRowIdx = rIdx
				break
			}
		}

		// 默认列映射退化逻辑
		if oidIdx == -1 && nameIdx == -1 {
			oidIdx = 0
			nameIdx = 1
			typeIdx = 2
			descIdx = 3
			headerRowIdx = 0
		} else if oidIdx == -1 {
			oidIdx = 0
		} else if nameIdx == -1 {
			nameIdx = 1
		}

		// 遍历数据行
		for rIdx, row := range rows {
			if rIdx <= headerRowIdx {
				continue
			}

			getCol := func(idx int) string {
				if idx >= 0 && idx < len(row) {
					return strings.TrimSpace(row[idx])
				}
				return ""
			}

			rawOid := ensureUtf8(getCol(oidIdx))
			rawName := ensureUtf8(getCol(nameIdx))
			rawType := ensureUtf8(getCol(typeIdx))
			rawDesc := ensureUtf8(getCol(descIdx))
			rawAccess := ensureUtf8(getCol(accessIdx))

			cleanOid := cleanOidString(rawOid)
			if cleanOid == "" && isValidOid(cleanOid) {
				continue
			}
			if cleanOid == "" && rawName == "" {
				continue
			}
			if !isValidOid(cleanOid) {
				// 如果 OID 字段不规范，尝试在 name 字段寻找 OID
				if isValidOid(cleanOidString(rawName)) {
					cleanOid = cleanOidString(rawName)
					rawName = rawOid
				} else {
					continue
				}
			}

			genericWords := map[string]bool{
				"the": true, "this": true, "that": true, "wait": true, "global": true,
				"configuration": true, "information": true, "a": true, "an": true,
				"table": true, "entry": true, "type": true, "name": true, "status": true,
			}
			rawNameLower := strings.ToLower(strings.TrimSpace(rawName))
			if rawName == "" || strings.HasPrefix(rawNameLower, "oid_") || genericWords[rawNameLower] {
				reIdent := regexp.MustCompile(`\b([a-zA-Z][a-zA-Z0-9_\-]{2,64})\b`)
				matches := reIdent.FindAllStringSubmatch(rawDesc, -1)
				foundSymbol := ""
				for _, m := range matches {
					if len(m) > 1 {
						w := m[1]
						wLower := strings.ToLower(w)
						if !genericWords[wLower] && len(w) >= 3 {
							foundSymbol = w
							break
						}
					}
				}
				if foundSymbol != "" {
					rawName = foundSymbol
				} else {
					rawName = "oid_" + strings.ReplaceAll(cleanOid, ".", "_")
				}
			}

			accessVal := rawAccess
			if accessVal == "" {
				accessVal = "read-only"
			}

			typeVal := rawType
			if typeVal == "" {
				typeVal = "OCTET STRING"
			}

			isTable := 0
			isScalar := 1
			typeLower := strings.ToLower(typeVal)
			nameLower := strings.ToLower(rawName)
			if strings.Contains(typeLower, "table") || strings.Contains(nameLower, "table") || strings.Contains(typeLower, "column") || strings.Contains(typeLower, "columnar") {
				isTable = 1
				isScalar = 0
			}

			allObjects = append(allObjects, repos.MibObject{
				ID:          uuid.NewString(),
				ModuleID:    moduleID,
				ModuleName:  moduleName,
				OID:         cleanOid,
				Name:        rawName,
				Syntax:      &typeVal,
				Access:      &accessVal,
				Status:      strPtr("current"),
				Description: strPtr(rawDesc),
				IsTable:     isTable,
				IsScalar:    isScalar,
			})
		}
	}

	if len(allObjects) == 0 {
		return fmt.Errorf("未在 Excel 中识别到有效的 OID 数据行 (需包含 OID 数字格式及指标名称)")
	}

	// 保存 Module
	desc := fmt.Sprintf("从 Excel 文件导入 (%d 个 OID)", len(allObjects))
	mod := &repos.MibModule{
		ID:          moduleID,
		Name:        moduleName,
		FileName:    &filename,
		Description: &desc,
		ObjectCount: len(allObjects),
		UploadedAt:  nowStr,
	}

	if err := repos.MibModules.Create(*mod); err != nil {
		return fmt.Errorf("创建 MIB 模块失败: %w", err)
	}

	// 批量保存 Objects
	if err := repos.MibObjects.CreateBatch(allObjects); err != nil {
		logger.Warnf("保存 MIB 对象失败: %v", err)
	}

	resp.SuccessCount++
	resp.Modules = append(resp.Modules, UploadResultItem{
		ID:          moduleID,
		Name:        moduleName,
		FileName:    filename,
		ObjectCount: len(allObjects),
	})

	return nil
}

// ZIP 解压安全上限，防止 ZIP 炸弹耗尽内存/磁盘
const (
	maxZipEntries    = 2000
	maxZipEntryBytes = 20 * 1024 * 1024  // 单文件 20MB
	maxZipTotalBytes = 200 * 1024 * 1024 // 总量 200MB
)

// parseAndSaveZip 解析 ZIP 压缩包中的 MIB 与 Excel
func parseAndSaveZip(filename string, content []byte, resp *UploadResponseData) error {
	r, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("无法读取 ZIP 压缩包: %w", err)
	}
	if len(r.File) > maxZipEntries {
		return fmt.Errorf("ZIP 包内文件数超过上限 (%d > %d)", len(r.File), maxZipEntries)
	}

	successCount := 0
	var totalBytes int64

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxZipEntryBytes {
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: 单文件解压后超过 %dMB，已跳过", f.Name, maxZipEntryBytes/(1024*1024)))
			continue
		}
		if totalBytes+int64(f.UncompressedSize64) > maxZipTotalBytes {
			return fmt.Errorf("ZIP 包解压总量超过上限 %dMB，已中止", maxZipTotalBytes/(1024*1024))
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		// 即使头部声明的尺寸被伪造，也用 LimitReader 限制实际读取量
		remaining := maxZipTotalBytes - totalBytes
		limit := int64(maxZipEntryBytes)
		if remaining < limit {
			limit = remaining
		}
		fContent, err := io.ReadAll(io.LimitReader(rc, limit+1))
		rc.Close()
		if err != nil {
			continue
		}
		if int64(len(fContent)) > limit {
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: 解压数据超过允许大小，已跳过", f.Name))
			continue
		}
		totalBytes += int64(len(fContent))

		subExt := strings.ToLower(filepath.Ext(f.Name))
		subName := filepath.Base(f.Name)

		switch subExt {
		case ".xlsx", ".xls":
			if err := parseAndSaveExcel(subName, fContent, resp); err == nil {
				successCount++
			}
		default:
			if err := parseAndSaveTextMib(subName, ensureUtf8(string(fContent)), resp); err == nil {
				successCount++
			}
		}
	}

	if successCount == 0 {
		return fmt.Errorf("ZIP 包内未找到有效的 MIB 或 Excel 文件")
	}

	return nil
}

// parseAndSaveTextMib 解析标准 MIB 文本文件
func parseAndSaveTextMib(filename string, content string, resp *UploadResponseData) error {
	moduleName := strings.TrimSuffix(filename, filepath.Ext(filename))

	// 正则提取 DEFINITIONS ::= BEGIN
	moduleReg := regexp.MustCompile(`(?i)^\s*([A-Za-z0-9\-_]+)\s+DEFINITIONS\s*::=\s*BEGIN`)
	if match := moduleReg.FindStringSubmatch(content); len(match) > 1 {
		moduleName = match[1]
	}

	moduleID := uuid.NewString()
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	var objects []repos.MibObject

	// 正则匹配 OBJECT-TYPE
	objReg := regexp.MustCompile(`(?s)([A-Za-z0-9\-_]+)\s+OBJECT-TYPE\s+SYNTAX\s+([^\n]+?)\s+(?:MAX-)?ACCESS\s+([^\n]+?)\s+STATUS\s+([^\n]+?)\s+DESCRIPTION\s+"([^"]*?)"\s*::=\s*\{\s*([A-Za-z0-9\-_]+)\s+([0-9]+)\s*\}`)
	matches := objReg.FindAllStringSubmatch(content, -1)

	for _, m := range matches {
		if len(m) < 8 {
			continue
		}
		name := ensureUtf8(strings.TrimSpace(m[1]))
		syntax := ensureUtf8(strings.TrimSpace(m[2]))
		access := ensureUtf8(strings.TrimSpace(m[3]))
		desc := ensureUtf8(strings.TrimSpace(m[5]))
		parent := strings.TrimSpace(m[6])
		subIdx := strings.TrimSpace(m[7])

		oid := fmt.Sprintf("1.3.6.1.4.1.%s.%s", parent, subIdx)

		objects = append(objects, repos.MibObject{
			ID:          uuid.NewString(),
			ModuleID:    moduleID,
			ModuleName:  moduleName,
			OID:         oid,
			Name:        name,
			Syntax:      &syntax,
			Access:      &access,
			Status:      strPtr("current"),
			Description: &desc,
			IsTable:     0,
			IsScalar:    1,
		})
	}

	if len(objects) == 0 {
		// 规则二退化匹配包含 OID ::= { 1 3 6 ... }
		lineReg := regexp.MustCompile(`([A-Za-z0-9\-_]+)\s+.*?\s*::=\s*\{\s*([0-9\s.]+)\s*\}`)
		lineMatches := lineReg.FindAllStringSubmatch(content, -1)
		for _, lm := range lineMatches {
			if len(lm) < 3 {
				continue
			}
			name := ensureUtf8(strings.TrimSpace(lm[1]))
			oidStr := cleanOidString(lm[2])
			if !isValidOid(oidStr) {
				continue
			}
			objects = append(objects, repos.MibObject{
				ID:          uuid.NewString(),
				ModuleID:    moduleID,
				ModuleName:  moduleName,
				OID:         oidStr,
				Name:        name,
				Syntax:      strPtr("OCTET STRING"),
				Access:      strPtr("read-only"),
				Status:      strPtr("current"),
				Description: strPtr("MIB 文件对象"),
				IsTable:     0,
				IsScalar:    1,
			})
		}
	}

	if len(objects) == 0 {
		return fmt.Errorf("未包含格式正确的 OBJECT-TYPE 定义")
	}

	desc := fmt.Sprintf("上传解包 MIB 模块 (%d 个对象)", len(objects))
	mod := &repos.MibModule{
		ID:          moduleID,
		Name:        moduleName,
		FileName:    &filename,
		FileContent: &content,
		Description: &desc,
		ObjectCount: len(objects),
		UploadedAt:  nowStr,
	}

	if err := repos.MibModules.Create(*mod); err != nil {
		return fmt.Errorf("创建 MIB 模块失败: %w", err)
	}

	if err := repos.MibObjects.CreateBatch(objects); err != nil {
		logger.Warnf("保存 MIB 对象失败: %v", err)
	}

	resp.SuccessCount++
	resp.Modules = append(resp.Modules, UploadResultItem{
		ID:          moduleID,
		Name:        moduleName,
		FileName:    filename,
		ObjectCount: len(objects),
	})

	return nil
}

func ensureUtf8(s string) string {
	if s == "" || utf8.ValidString(s) {
		return s
	}
	dec := simplifiedchinese.GB18030.NewDecoder()
	utf8Str, _, err := transform.String(dec, s)
	if err == nil && utf8.ValidString(utf8Str) && !strings.Contains(utf8Str, "\ufffd") {
		return utf8Str
	}
	return s
}

func cleanOidString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", ".")
	s = strings.TrimPrefix(s, ".")
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return s
}

func isValidOid(oid string) bool {
	if oid == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[0-9]+(\.[0-9]+)+$`, oid)
	return matched
}

func strPtr(s string) *string {
	return &s
}
