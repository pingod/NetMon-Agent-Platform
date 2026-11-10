package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"itops-backend/internal/appdb"
	"itops-backend/internal/middleware"
	"itops-backend/internal/migrate"
	"itops-backend/internal/repos"
)

func setupTestDB(t *testing.T) func() {
	tmpDb := "test_server_features.db"
	_ = os.Remove(tmpDb)

	if err := appdb.Open(tmpDb); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := migrate.RunMigrations(appdb.DB()); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	appdb.InitServersTableSchema(appdb.DB())

	return func() {
		appdb.Close()
		_ = os.Remove(tmpDb)
	}
}

// 1. 测试服务器添加 (POST /api/v1/servers)
func TestFeature_ServerAdd(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	r := ServerRoutes()

	// 构造添加服务器请求
	payload := repos.ServerCreateInput{
		Name:        "Test Server Add",
		Hostname:    "192.168.10.100",
		Port:        22,
		Username:    "root",
		Password:    "P@ssw0rd123",
		OSType:      "linux",
		Vendor:      "dell",
		Model:       "PowerEdge R750",
		Description: "Primary Application Node",
		Tags:        []string{"prod", "app"},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.SetUser(req, &middleware.AuthUser{ID: "1", Username: "admin", Role: "admin", Enabled: 1})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("Expected 200/201 on add server, got %d, body: %s", w.Code, w.Body.String())
	}

	type ServerRespData struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Hostname string   `json:"hostname"`
		Port     int      `json:"port"`
		Vendor   *string  `json:"vendor"`
		Model    *string  `json:"model"`
		Tags     []string `json:"tags"`
	}

	var resp struct {
		Success bool            `json:"success"`
		Data    *ServerRespData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp.Data == nil || resp.Data.ID == "" {
		t.Fatalf("Expected created server data with ID, got %+v", resp.Data)
	}
	if resp.Data.Name != "Test Server Add" || *resp.Data.Vendor != "dell" {
		t.Errorf("Unexpected server attributes: %+v", resp.Data)
	}
	t.Logf("✅ 服务器添加功能正常: ID=%s, Name=%s, Vendor=%v", resp.Data.ID, resp.Data.Name, *resp.Data.Vendor)
}

// 2. 测试服务器编辑 (PUT /api/v1/servers/{id})
func TestFeature_ServerEdit(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 先创建一台服务器
	created, err := repos.Servers.Create(repos.ServerCreateInput{
		Name:     "Old Name",
		Hostname: "192.168.10.101",
		Port:     22,
		Username: "root",
		Password: "InitialPassword",
		OSType:   "linux",
		Vendor:   "other",
	})
	if err != nil {
		t.Fatalf("Failed to create initial server: %v", err)
	}

	r := ServerRoutes()

	// 构造编辑更新请求
	newName := "Updated Name Server"
	newPort := 2222
	newVendor := "huawei"
	newModel := "TaiShan 200"
	newDesc := "Updated Description"
	newTags := []string{"updated", "k8s"}
	updatePayload := repos.ServerUpdateInput{
		Name:        &newName,
		Port:        &newPort,
		Vendor:      &newVendor,
		Model:       &newModel,
		Description: &newDesc,
		Tags:        &newTags,
	}
	body, _ := json.Marshal(updatePayload)

	req := httptest.NewRequest(http.MethodPut, "/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.SetUser(req, &middleware.AuthUser{ID: "1", Username: "admin", Role: "admin", Enabled: 1})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on edit server, got %d, body: %s", w.Code, w.Body.String())
	}

	type ServerRespData struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Hostname string   `json:"hostname"`
		Port     int      `json:"port"`
		Vendor   *string  `json:"vendor"`
		Model    *string  `json:"model"`
		Tags     []string `json:"tags"`
	}

	var editResp struct {
		Success bool            `json:"success"`
		Data    *ServerRespData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &editResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if editResp.Data == nil || editResp.Data.Name != "Updated Name Server" {
		t.Fatalf("Expected updated server name, got %+v", editResp.Data)
	}
	if editResp.Data.Port != 2222 || *editResp.Data.Vendor != "huawei" || *editResp.Data.Model != "TaiShan 200" {
		t.Errorf("Unexpected updated attributes: %+v", editResp.Data)
	}
	t.Logf("✅ 服务器编辑功能正常: Name=%s, Port=%d, Vendor=%v, Model=%v", editResp.Data.Name, editResp.Data.Port, *editResp.Data.Vendor, *editResp.Data.Model)
}

// 3. 测试批量导入 (JSON & CSV 导入)
func TestFeature_ServerBatchImport(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 3.1 测试 JSON 批量导入 (POST /api/v1/server-management/import)
	smRouter := ServerManagementRoutes()

	jsonImportPayload := map[string]interface{}{
		"servers": []map[string]interface{}{
			{
				"name":        "JSON Import Node 1",
				"hostname":    "192.168.10.111",
				"port":        22,
				"username":    "root",
				"password":    "Pwd111",
				"os_type":     "linux",
				"description": "Batch 1",
			},
			{
				"name":        "JSON Import Node 2",
				"hostname":    "192.168.10.112",
				"port":        22,
				"username":    "root",
				"password":    "Pwd112",
				"os_type":     "linux",
				"description": "Batch 2",
			},
		},
		"test_connection": false,
	}
	jsonBody, _ := json.Marshal(jsonImportPayload)

	reqJson := httptest.NewRequest(http.MethodPost, "/import", bytes.NewReader(jsonBody))
	reqJson.Header.Set("Content-Type", "application/json")
	reqJson = middleware.SetUser(reqJson, &middleware.AuthUser{ID: "1", Username: "admin", Role: "admin", Enabled: 1})
	wJson := httptest.NewRecorder()

	smRouter.ServeHTTP(wJson, reqJson)

	if wJson.Code != http.StatusOK {
		t.Fatalf("Expected 200 on JSON import, got %d, body: %s", wJson.Code, wJson.Body.String())
	}

	var jsonResp struct {
		Success bool `json:"success"`
		Data    struct {
			Success int `json:"success"`
			Failed  int `json:"failed"`
			Skipped int `json:"skipped"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wJson.Body.Bytes(), &jsonResp)
	if jsonResp.Data.Success != 2 {
		t.Fatalf("Expected 2 success imports, got %+v", jsonResp.Data)
	}
	t.Logf("✅ JSON 批量导入正常: 成功=%d, 失败=%d, 跳过=%d", jsonResp.Data.Success, jsonResp.Data.Failed, jsonResp.Data.Skipped)

	// 3.2 测试 CSV 批量导入 (POST /api/v1/import-export/servers/import)
	ieRouter := ImportExportRoutes()

	csvContent := `名称,IP地址,端口,用户名,密码,私钥,使用密钥,描述,标签,系统类型,分组
CSV Node 1,192.168.10.121,22,root,CsvPwd1,,否,CSV导入节点1,csv,linux,
CSV Node 2,192.168.10.122,22,root,CsvPwd2,,否,CSV导入节点2,csv,linux,
`
	csvImportPayload := map[string]string{
		"csvContent": csvContent,
	}
	csvBody, _ := json.Marshal(csvImportPayload)

	reqCsv := httptest.NewRequest(http.MethodPost, "/servers/import", bytes.NewReader(csvBody))
	reqCsv.Header.Set("Content-Type", "application/json")
	reqCsv = middleware.SetUser(reqCsv, &middleware.AuthUser{ID: "1", Username: "admin", Role: "admin", Enabled: 1})
	wCsv := httptest.NewRecorder()

	ieRouter.ServeHTTP(wCsv, reqCsv)

	if wCsv.Code != http.StatusOK {
		t.Fatalf("Expected 200 on CSV import, got %d, body: %s", wCsv.Code, wCsv.Body.String())
	}

	var csvResp struct {
		Success bool `json:"success"`
		Data    struct {
			Imported int      `json:"imported"`
			Failed   int      `json:"failed"`
			Errors   []string `json:"errors"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wCsv.Body.Bytes(), &csvResp)
	if csvResp.Data.Imported != 2 {
		t.Fatalf("Expected 2 CSV imports, got %+v", csvResp.Data)
	}
	t.Logf("✅ CSV 批量导入正常: 成功导入=%d, 失败=%d", csvResp.Data.Imported, csvResp.Data.Failed)
}

// 4. 测试批量巡检接口调度与报告生成 (POST /api/v1/servers/batch-inspect)
func TestFeature_ServerBatchInspect(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 创建两台受检服务器
	s1, _ := repos.Servers.Create(repos.ServerCreateInput{
		Name:     "Inspect Server 1",
		Hostname: "192.168.10.201",
		Port:     22,
		Username: "root",
		Password: "InspectPwd1",
		OSType:   "linux",
	})
	s2, _ := repos.Servers.Create(repos.ServerCreateInput{
		Name:     "Inspect Server 2",
		Hostname: "192.168.10.202",
		Port:     22,
		Username: "root",
		Password: "InspectPwd2",
		OSType:   "linux",
	})

	r := ServerRoutes()

	// 构造批量巡检请求
	useAI := false
	concurrency := 2
	inspectPayload := map[string]interface{}{
		"server_ids":     []string{s1.ID, s2.ID},
		"inspectionType": "standard",
		"useAI":          useAI,
		"concurrency":    concurrency,
	}
	body, _ := json.Marshal(inspectPayload)

	req := httptest.NewRequest(http.MethodPost, "/batch-inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.SetUser(req, &middleware.AuthUser{ID: "1", Username: "admin", Role: "admin", Enabled: 1})
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on batch inspect, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ReportID     string                 `json:"report_id"`
			Total        int                    `json:"total"`
			SuccessCount int                    `json:"success_count"`
			FailedCount  int                    `json:"failed_count"`
			Results      map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Data.Total != 2 || resp.Data.ReportID == "" {
		t.Fatalf("Unexpected batch inspection response: %+v", resp.Data)
	}

	// 验证巡检报告是否已成功持久化入库 reports 表
	var reportCount int
	err := appdb.DB().QueryRow("SELECT COUNT(*) FROM reports WHERE id = ?", resp.Data.ReportID).Scan(&reportCount)
	if err != nil || reportCount != 1 {
		t.Fatalf("Report record not found in reports table: count=%d, err=%v", reportCount, err)
	}

	t.Logf("✅ 批量巡检接口与报告生成正常: Total=%d, ReportID=%s, ReportsTableVerified=true", resp.Data.Total, resp.Data.ReportID)
}
