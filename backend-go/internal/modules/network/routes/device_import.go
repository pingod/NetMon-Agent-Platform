// 网络设备批量导入 — 模板下载 + Excel 导入
// 等价于 TS networkDeviceRoutes.ts 中 /import/template 和 /import 端点
package routes

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/modules/network/services"
)

// 合法厂商列表（与 TS VALID_VENDORS 一致）
var validVendors = []string{
	"huawei", "cisco", "h3c", "ruijie", "zte", "fortinet", "paloalto",
	"juniper", "arista", "hpe", "mikrotik", "ubiquiti", "dell", "tplink", "f5", "ruijie_eg",
}

// 厂商别名映射（与 TS VENDOR_ALIASES 一致）
var vendorAliases = map[string]string{
	"华为": "huawei", "华三": "h3c", "思科": "cisco", "锐捷": "ruijie", "中兴": "zte",
	"迈普": "mikrotik", "深信服": "fortinet", "飞塔": "fortinet",
	"hw": "huawei", "h3": "h3c", "rj": "ruijie",
}

// 列名映射（中文/英文/别名 → 标准字段）
var columnMap = map[string]string{
	"名称": "name", "name": "name", "设备名": "name", "设备名称": "name", "hostname": "name",
	"IP": "ip_address", "ip": "ip_address", "ip地址": "ip_address", "IP地址": "ip_address", "ip_address": "ip_address", "管理IP": "ip_address",
	"厂商": "vendor", "vendor": "vendor", "品牌": "vendor", "厂牌": "vendor",
	"型号": "model", "model": "model", "设备型号": "model",
	"版本": "os_version", "os_version": "os_version", "系统版本": "os_version", "固件版本": "os_version",
	"SSH端口": "ssh_port", "ssh_port": "ssh_port", "端口": "ssh_port", "port": "ssh_port",
	"用户名": "username", "username": "username", "登录用户": "username",
	"密码": "password", "password": "password", "登录密码": "password",
	"Enable密码": "enable_password", "enable_password": "enable_password", "特权密码": "enable_password",
	"位置": "location", "location": "location", "机房": "location", "位置信息": "location",
	"角色": "role", "role": "role", "设备角色": "role",
	"SNMP启用": "snmp_enabled", "snmp_enabled": "snmp_enabled", "snmp": "snmp_enabled",
	"SNMP端口": "snmp_port", "snmp_port": "snmp_port",
	"SNMP只读串": "snmp_community", "snmp_community": "snmp_community", "SNMP团体字": "snmp_community", "只读串": "snmp_community", "community": "snmp_community",
	"SNMP版本": "snmp_version", "snmp_version": "snmp_version",
}

// normalizeVendor 将用户输入的厂商名标准化
func normalizeVendor(v string) string {
	lower := strings.ToLower(strings.TrimSpace(v))

	// 直接匹配合法列表
	for _, valid := range validVendors {
		if lower == valid {
			return valid
		}
	}

	// 匹配别名
	if alias, ok := vendorAliases[lower]; ok {
		return alias
	}

	// 原始值匹配别名
	if alias, ok := vendorAliases[v]; ok {
		return alias
	}

	return ""
}

// handleDownloadTemplate 下载导入模板
func handleDownloadTemplate(w http.ResponseWriter, _ *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "网络设备"
	f.SetSheetName("Sheet1", sheet)

	// 表头（13 列：基础信息 + SSH认证 + SNMP配置）
	headers := []string{"名称", "IP地址", "厂商", "型号", "SSH端口", "用户名", "密码", "Enable密码", "位置", "SNMP启用", "SNMP端口", "SNMP只读串", "SNMP版本"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// 示例数据（3 行）
	examples := [][]interface{}{
		{"核心交换机-1", "10.0.0.1", "huawei", "S5735", 22, "admin", "Admin@123", "Enable@123", "机房A", "是", 161, "public", "v2c"},
		{"汇聚交换机-1", "10.1.1.1", "h3c", "S5560", 22, "admin", "Admin@123", "", "机房B", "是", 161, "public", "v2c"},
		{"防火墙-1", "10.0.0.254", "cisco", "ASA5525", 22, "admin", "Admin@123", "", "机房A", "否", 161, "", ""},
	}
	for rowIdx, row := range examples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, val)
		}
	}

	// 列宽
	widths := []float64{16, 16, 10, 12, 10, 12, 14, 14, 10, 10, 10, 14, 10}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, w)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		logger.Errorf("生成导入模板失败: %v", err)
		httpx.RespondError(w, "生成模板失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=network_devices_template.xlsx")
	w.Write(buf.Bytes())
}

// handleBatchImport 批量导入网络设备
func handleBatchImport(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("handleBatchImport panic: %v", rec)
			httpx.RespondError(w, fmt.Sprintf("批量导入处理异常: %v", rec), http.StatusInternalServerError)
		}
	}()

	// 限制文件大小 20MB
	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024)

	// 解析 multipart
	if err := r.ParseMultipartForm(20 * 1024 * 1024); err != nil {
		httpx.RespondError(w, "文件解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.RespondError(w, "未找到上传文件，字段名需为 file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		httpx.RespondError(w, "仅支持 Excel(.xlsx/.xls) 或 CSV 文件", http.StatusBadRequest)
		return
	}

	var rows []map[string]string

	if ext == ".csv" {
		rows, err = parseCSV(file)
	} else {
		rows, err = parseExcel(file)
	}
	if err != nil {
		httpx.RespondError(w, "文件解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(rows) == 0 {
		httpx.RespondError(w, "文件中没有数据行", http.StatusBadRequest)
		return
	}

	// 逐行解析并校验
	type parseError struct {
		Row   int    `json:"row"`
		Error string `json:"error"`
	}

	var devices []services.CreateDeviceRequest
	var parseErrors []parseError
	var skipped []map[string]interface{}

	for i, row := range rows {
		rowNum := i + 2 // Excel 行号（第1行是表头）

		name := strings.TrimSpace(row["name"])
		ip := strings.TrimSpace(row["ip_address"])
		vendorRaw := strings.TrimSpace(row["vendor"])

		if name == "" || ip == "" {
			parseErrors = append(parseErrors, parseError{
				Row:   rowNum,
				Error: fmt.Sprintf("名称或IP为空 (name=%q, ip=%q)", name, ip),
			})
			continue
		}

		vendor := normalizeVendor(vendorRaw)
		if vendor == "" {
			parseErrors = append(parseErrors, parseError{
				Row:   rowNum,
				Error: fmt.Sprintf("不支持的厂商: %q", vendorRaw),
			})
			continue
		}

		sshPort := 22
		if v := strings.TrimSpace(row["ssh_port"]); v != "" {
			if n, err := parseInt(v); err == nil && n > 0 {
				sshPort = n
			}
		}

		snmpEnabled := 1
		if v := strings.TrimSpace(row["snmp_enabled"]); v != "" {
			if v == "0" || strings.ToLower(v) == "false" || v == "否" {
				snmpEnabled = 0
			}
		}

		snmpPort := 161
		if v := strings.TrimSpace(row["snmp_port"]); v != "" {
			if n, err := parseInt(v); err == nil && n > 0 {
				snmpPort = n
			}
		}

		devices = append(devices, services.CreateDeviceRequest{
			Name:           name,
			IPAddress:      ip,
			Vendor:         vendor,
			Model:          strings.TrimSpace(row["model"]),
			OsVersion:      strings.TrimSpace(row["os_version"]),
			SSHPort:        sshPort,
			Username:       strings.TrimSpace(row["username"]),
			Password:       strings.TrimSpace(row["password"]),
			EnablePassword: strings.TrimSpace(row["enable_password"]),
			Location:       strings.TrimSpace(row["location"]),
			Role:           strings.TrimSpace(row["role"]),
			SnmpEnabled:    snmpEnabled,
			SnmpPort:       snmpPort,
			SnmpCommunity:  strings.TrimSpace(row["snmp_community"]),
			SnmpVersion:    strings.TrimSpace(row["snmp_version"]),
		})
	}

	// 批量创建
	result := services.NetworkDeviceServiceInstance.BatchCreateDevices(devices)

	// 将 skipped 从结构体转为 map
	var skippedResult []map[string]interface{}
	for _, s := range result.Skipped {
		skippedResult = append(skippedResult, map[string]interface{}{
			"name":       s.Name,
			"ip_address": s.IPAddress,
			"reason":     s.Reason,
		})
	}
	if skippedResult == nil {
		skippedResult = []map[string]interface{}{}
	}

	// 将 failed 从结构体转为 map
	var failedResult []map[string]interface{}
	for _, f := range result.Failed {
		failedResult = append(failedResult, map[string]interface{}{
			"name":       f.Name,
			"ip_address": f.IPAddress,
			"error":      f.Error,
		})
	}
	if failedResult == nil {
		failedResult = []map[string]interface{}{}
	}

	// 合并路由层的 skipped（parseErrors 也作为 skipped 返回）
	for _, pe := range skipped {
		skippedResult = append(skippedResult, pe)
	}

	httpx.Respond(w, map[string]interface{}{
		"total":       len(rows),
		"parsed":      len(devices),
		"parseErrors": parseErrors,
		"imported":    len(result.Success),
		"skipped":     skippedResult,
		"failed":      failedResult,
		"details":     result,
	})
}

// parseExcel 解析 xlsx/xls 文件
func parseExcel(file multipart.File) ([]map[string]string, error) {
	buf, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取上传文件内容失败: %w", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("打开Excel失败: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel中没有工作表")
	}

	sheet := sheets[0]
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}

	if len(rows) < 2 {
		return nil, nil
	}

	// 第一行是表头
	headers := rows[0]
	headerMap := make([]string, len(headers))
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if mapped, ok := columnMap[h]; ok {
			headerMap[i] = mapped
		} else {
			headerMap[i] = h
		}
	}

	var result []map[string]string
	for _, row := range rows[1:] {
		m := make(map[string]string)
		for colIdx, cellVal := range row {
			if colIdx < len(headerMap) {
				m[headerMap[colIdx]] = cellVal
			}
		}
		result = append(result, m)
	}

	return result, nil
}

// parseCSV 解析 csv 文件
func parseCSV(file multipart.File) ([]map[string]string, error) {
	buf, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取上传文件内容失败: %w", err)
	}
	reader := csv.NewReader(bytes.NewReader(buf))

	// 读取表头
	headerRow, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取CSV表头失败: %w", err)
	}

	var headers []string
	for _, h := range headerRow {
		h = strings.TrimSpace(h)
		if mapped, ok := columnMap[h]; ok {
			headers = append(headers, mapped)
		} else {
			headers = append(headers, h)
		}
	}

	var result []map[string]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		m := make(map[string]string)
		for colIdx, cellVal := range record {
			if colIdx < len(headers) {
				m[headers[colIdx]] = strings.TrimSpace(cellVal)
			}
		}
		result = append(result, m)
	}

	return result, nil
}

// parseInt 安全解析整数
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
