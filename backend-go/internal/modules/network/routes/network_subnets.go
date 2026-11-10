// 网络子网路由 — 8 个端点
// 等价于 TS routes/networkSubnetRoutes.ts
package routes

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// NetworkSubnetRoutes 网络子网路由
func NetworkSubnetRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 获取所有子网
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		subnets, err := repos.NetworkSubnets.List()
		if err != nil {
			httpx.RespondError(w, "获取子网列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, subnets)
	})

	// GET /:id — 获取单个子网
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		subnet, err := repos.NetworkSubnets.GetByID(chi.URLParam(r, "id"))
		if err != nil || subnet == nil {
			httpx.RespondError(w, "子网不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, subnet)
	})

	// POST / — 创建子网
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string `json:"name"`
			CIDR        string `json:"cidr"`
			Gateway     string `json:"gateway"`
			VlanID      int    `json:"vlan_id"`
			NetworkType string `json:"network_type"`
			Location    string `json:"location"`
			Description string `json:"description"`
			Status      string `json:"status"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.CIDR == "" {
			httpx.RespondError(w, "名称和CIDR不能为空", http.StatusBadRequest)
			return
		}

		total := cidrToTotalIPs(req.CIDR)
		id := uuid.New().String()

		if req.NetworkType == "" {
			req.NetworkType = "lan"
		}
		if req.Status == "" {
			req.Status = "active"
		}

		err := repos.NetworkSubnets.Create(repos.SubnetCreateInput{
			ID:          id,
			Name:        req.Name,
			CIDR:        req.CIDR,
			Gateway:     req.Gateway,
			VlanID:      req.VlanID,
			NetworkType: req.NetworkType,
			Location:    req.Location,
			Description: req.Description,
			Status:      req.Status,
			TotalIPs:    total,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 自动生成 IP 地址池
		if total > 0 && total <= 65536 {
			ips := cidrToIPList(req.CIDR, total)
			if len(ips) > 0 {
				_ = repos.NetworkIPs.BulkInsertAvailable(id, ips)
			}
		}

		httpx.Respond(w, map[string]string{"id": id})
	})

	// PUT /:id — 更新子网
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        *string `json:"name"`
			Gateway     *string `json:"gateway"`
			VlanID      *int    `json:"vlan_id"`
			NetworkType *string `json:"network_type"`
			Location    *string `json:"location"`
			Description *string `json:"description"`
			Status      *string `json:"status"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		err := repos.NetworkSubnets.Update(chi.URLParam(r, "id"), repos.SubnetUpdateInput{
			Name:        req.Name,
			Gateway:     req.Gateway,
			VlanID:      req.VlanID,
			NetworkType: req.NetworkType,
			Location:    req.Location,
			Description: req.Description,
			Status:      req.Status,
		})
		if err != nil {
			httpx.RespondError(w, "更新子网失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /:id — 删除子网
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.NetworkSubnets.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除子网失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// GET /:id/ips — 获取子网下所有 IP
	r.Get("/{id}/ips", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(q.Get("pageSize"))
		if pageSize < 10 {
			pageSize = 100
		}
		if pageSize > 500 {
			pageSize = 500
		}
		offset := (page - 1) * pageSize

		filters := repos.IPListFilters{
			SubnetID: chi.URLParam(r, "id"),
			Status:   q.Get("status"),
			Search:   q.Get("search"),
			Limit:    pageSize,
			Offset:   offset,
		}
		if filters.Status == "all" {
			filters.Status = ""
		}

		ips, err := repos.NetworkIPs.List(filters)
		if err != nil {
			httpx.RespondError(w, "获取IP列表失败", http.StatusInternalServerError)
			return
		}
		total, _ := repos.NetworkIPs.Count(filters)
		stats, _ := repos.NetworkIPs.StatsByStatus(chi.URLParam(r, "id"))

		httpx.Respond(w, map[string]interface{}{
			"ips":      ips,
			"stats":    stats,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
	})

	// PUT /:id/ips/:ipId — 更新单个 IP
	r.With(middleware.RequireRole("admin")).Put("/{id}/ips/{ipId}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Status      *string `json:"status"`
			DeviceName  *string `json:"device_name"`
			MacAddress  *string `json:"mac_address"`
			Description *string `json:"description"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		err := repos.NetworkIPs.Update(chi.URLParam(r, "ipId"), chi.URLParam(r, "id"), repos.IPUpdateInput{
			Status:      req.Status,
			DeviceName:  req.DeviceName,
			MacAddress:  req.MacAddress,
			Description: req.Description,
		})
		if err != nil {
			httpx.RespondError(w, "更新IP失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// POST /:id/ips/batch — 批量分配/释放 IP
	r.With(middleware.RequireRole("admin")).Post("/{id}/ips/batch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IPIDs       []string `json:"ip_ids"`
			Status      string   `json:"status"`
			DeviceName  string   `json:"device_name"`
			Description string   `json:"description"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.IPIDs) == 0 {
			httpx.RespondError(w, "请选择IP地址", http.StatusBadRequest)
			return
		}

		err := repos.NetworkIPs.BatchUpdate(req.IPIDs, chi.URLParam(r, "id"), repos.IPBatchUpdateInput{
			Status:      req.Status,
			DeviceName:  req.DeviceName,
			Description: req.Description,
		})
		if err != nil {
			httpx.RespondError(w, "批量操作失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]int{"count": len(req.IPIDs)})
	})

	// GET /:id/ips/template — 下载 IP 导入模板
	r.With(middleware.RequireRole("admin")).Get("/{id}/ips/template", func(w http.ResponseWriter, r *http.Request) {
		f := excelize.NewFile()
		defer f.Close()
		sheet := "IP导入模板"
		f.SetSheetName("Sheet1", sheet)

		headers := []string{"IP地址", "状态", "设备名", "MAC地址", "说明"}
		for i, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			_ = f.SetCellValue(sheet, cell, h)
		}
		// 示例行
		example := []string{"192.168.0.2", "可用", "财务部打印机", "AA:BB:CC:DD:EE:FF", "前台共享打印机"}
		for i, v := range example {
			cell, _ := excelize.CoordinatesToCellName(i+1, 2)
			_ = f.SetCellValue(sheet, cell, v)
		}
		// 表头样式
		styleID, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"D9E1F2"}, Pattern: 1},
		})
		_ = f.SetCellStyle(sheet, "A1", "E1", styleID)
		// 列宽
		_ = f.SetColWidth(sheet, "A", "E", 22)

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", "attachment; filename=ip_import_template.xlsx")
		_ = f.Write(w)
	})

	// POST /:id/ips/import — Excel/CSV 导入 IP
	r.With(middleware.RequireRole("admin")).Post("/{id}/ips/import", func(w http.ResponseWriter, r *http.Request) {
		subnetID := chi.URLParam(r, "id")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			httpx.RespondError(w, "解析上传文件失败", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			httpx.RespondError(w, "请上传 Excel 文件", http.StatusBadRequest)
			return
		}
		defer file.Close()

		rows, perr := parseIPExcel(file)
		if perr != nil {
			httpx.RespondError(w, "解析Excel失败: "+perr.Error(), http.StatusBadRequest)
			return
		}
		if len(rows) == 0 {
			httpx.RespondError(w, "Excel 中没有数据行", http.StatusBadRequest)
			return
		}

		items := make([]repos.IPImportItem, 0, len(rows))
		var parseErrors []string
		for i, row := range rows {
			ip := strings.TrimSpace(row["ip_address"])
			if ip == "" {
				parseErrors = append(parseErrors, fmt.Sprintf("第 %d 行: IP地址为空", i+2))
				continue
			}
			items = append(items, repos.IPImportItem{
				IPAddress:   ip,
				Status:      row["status"],
				DeviceName:  row["device_name"],
				MacAddress:  row["mac_address"],
				Description: row["description"],
			})
		}

		if len(items) == 0 {
			httpx.RespondError(w, "没有有效数据可导入", http.StatusBadRequest)
			return
		}

		inserted, updated, ierr := repos.NetworkIPs.ImportByIP(subnetID, items)
		if ierr != nil {
			httpx.RespondError(w, "导入失败: "+ierr.Error(), http.StatusInternalServerError)
			return
		}

		httpx.Respond(w, map[string]interface{}{
			"total":       len(rows),
			"imported":    inserted,
			"updated":     updated,
			"parseErrors": parseErrors,
		})
	})

	return r
}

// ipImportColumnMap IP 导入列名映射（中文/英文 → 标准字段）
var ipImportColumnMap = map[string]string{
	"ip": "ip_address", "ip地址": "ip_address", "IP地址": "ip_address", "IP": "ip_address",
	"ip_address": "ip_address", "地址": "ip_address",
	"状态": "status", "status": "status",
	"设备": "device_name", "设备名": "device_name", "设备名称": "device_name",
	"device_name": "device_name", "设备名称/主机名": "device_name", "主机名": "device_name", "hostname": "device_name",
	"mac": "mac_address", "mac地址": "mac_address", "MAC地址": "mac_address", "MAC": "mac_address",
	"mac_address": "mac_address", "物理地址": "mac_address",
	"说明": "description", "备注": "description", "描述": "description",
	"description": "description", "备注说明": "description",
}

// parseIPExcel 解析 IP 导入的 Excel/CSV 文件（第一行表头）
func parseIPExcel(file multipart.File) ([]map[string]string, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("打开Excel失败: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel中没有工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil
	}

	headers := rows[0]
	headerMap := make([]string, len(headers))
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if mapped, ok := ipImportColumnMap[h]; ok {
			headerMap[i] = mapped
		} else {
			headerMap[i] = strings.ToLower(h)
		}
	}

	var result []map[string]string
	for _, row := range rows[1:] {
		m := make(map[string]string)
		for colIdx, cellVal := range row {
			if colIdx < len(headerMap) {
				m[headerMap[colIdx]] = strings.TrimSpace(cellVal)
			}
		}
		result = append(result, m)
	}
	return result, nil
}

// cidrToTotalIPs 计算 CIDR 中可用 IP 数量
func cidrToTotalIPs(cidr string) int {
	parts := strings.SplitN(cidr, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	prefix, err := strconv.Atoi(parts[1])
	if err != nil || prefix < 0 || prefix > 32 {
		return 0
	}
	total := (1 << (32 - prefix)) - 2
	if total < 0 {
		total = 0
	}
	return total
}

// cidrToIPList 生成 CIDR 范围内可用 IP 列表
func cidrToIPList(cidr string, total int) []string {
	parts := strings.SplitN(cidr, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	ipInt := ipToInt(parts[0])
	if ipInt == 0 {
		return nil
	}

	result := make([]string, 0, total)
	for i := 1; i <= total; i++ {
		result = append(result, intToIP(ipInt+uint32(i)))
	}
	return result
}

func ipToInt(ip string) uint32 {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0
	}
	var result uint32
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || v > 255 {
			return 0
		}
		result = (result << 8) | uint32(v)
	}
	return result
}

func intToIP(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		(n>>24)&0xFF,
		(n>>16)&0xFF,
		(n>>8)&0xFF,
		n&0xFF,
	)
}
