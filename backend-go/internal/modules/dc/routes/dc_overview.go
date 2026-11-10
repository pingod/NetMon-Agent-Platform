// DC 概览 / 设备分布 / 生命周期 / 导出 / 导入 处理函数
package dcroutes

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 概览
// ───────────────────────────────────────────────

// DcOverviewData 概览数据结构（与前端 OverviewData 对齐）
type DcOverviewData struct {
	Summary       DcOverviewSummary      `json:"summary"`
	RackData      []DcRackAlert           `json:"rackData"`
	Rooms         []repos.DcRoom          `json:"rooms"`
	IsEmpty       bool                    `json:"isEmpty"`
	IsPartialMock bool                    `json:"isPartialMock"`
	Pue           float64                 `json:"pue"`
	TotalPowerKw  float64                 `json:"totalPowerKw"`
}

// DcOverviewSummary 概览汇总
type DcOverviewSummary struct {
	TotalRooms     int     `json:"totalRooms"`
	TotalRacks     int     `json:"totalRacks"`
	TotalDevices   int     `json:"totalDevices"`
	OnlineDevices  int     `json:"onlineDevices"`
	AlertDevices   int     `json:"alertDevices"`
	OfflineDevices int     `json:"offlineDevices"`
	AvgTemp        float64 `json:"avgTemp"`
	AvgHumidity    float64 `json:"avgHumidity"`
}

// DcRackAlert 机柜告警状态（前端 rackData 用）
type DcRackAlert struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlertCount int    `json:"alert_count"`
	Status     string `json:"status"`
}

func handleOverview(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	// 汇总统计
	var totalRooms, totalRacks, totalDevices, onlineDevices, alertDevices, offlineDevices int
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_rooms`).Scan(&totalRooms)
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_racks`).Scan(&totalRacks)
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_rack_slots`).Scan(&totalDevices)
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_rack_slots s WHERE s.device_type = 'server' AND EXISTS (
		SELECT 1 FROM servers sv WHERE sv.id = s.device_id AND sv.status = 'online')`).Scan(&onlineDevices)
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_racks WHERE status IN ('warning','critical')`).Scan(&alertDevices)
	_ = db.QueryRow(`SELECT COUNT(*) FROM dc_rack_slots s WHERE s.device_type = 'server' AND EXISTS (
		SELECT 1 FROM servers sv WHERE sv.id = s.device_id AND sv.status = 'offline')`).Scan(&offlineDevices)

	// 温度/湿度均值
	var avgTemp, avgHum float64
	_ = db.QueryRow(`SELECT COALESCE(AVG(current_temperature), 0) FROM dc_rooms`).Scan(&avgTemp)
	_ = db.QueryRow(`SELECT COALESCE(AVG(current_humidity), 0) FROM dc_rooms`).Scan(&avgHum)

	// 机柜告警状态
	rackRows, err := db.Query(`
		SELECT r.id, r.name, r.status,
		       (SELECT COUNT(*) FROM dc_rack_slots s WHERE s.rack_id = r.id AND s.device_type = 'server'
		        AND EXISTS (SELECT 1 FROM servers sv WHERE sv.id = s.device_id AND sv.status = 'warning')) AS alert_count
		FROM dc_racks r`)
	if err == nil {
		defer rackRows.Close()
	}
	var rackData []DcRackAlert
	for rackRows != nil && rackRows.Next() {
		var ra DcRackAlert
		if err := rackRows.Scan(&ra.ID, &ra.Name, &ra.Status, &ra.AlertCount); err != nil {
			continue
		}
		rackData = append(rackData, ra)
	}
	if rackData == nil {
		rackData = []DcRackAlert{}
	}

	rooms, _ := repos.DcRooms.List()

	// PUE 与总功率（机房合计）
	var pue, totalPower float64
	if totalRooms > 0 {
		_ = db.QueryRow(`SELECT COALESCE(AVG(pue), 1.45) FROM dc_rooms`).Scan(&pue)
		_ = db.QueryRow(`SELECT COALESCE(SUM(total_power_kw), 0) FROM dc_rooms`).Scan(&totalPower)
	} else {
		pue = 1.45
	}

	httpx.Respond(w, DcOverviewData{
		Summary: DcOverviewSummary{
			TotalRooms:     totalRooms,
			TotalRacks:     totalRacks,
			TotalDevices:   totalDevices,
			OnlineDevices:  onlineDevices,
			AlertDevices:   alertDevices,
			OfflineDevices: offlineDevices,
			AvgTemp:        avgTemp,
			AvgHumidity:    avgHum,
		},
		RackData:      rackData,
		Rooms:         rooms,
		IsEmpty:       totalRooms == 0 && totalRacks == 0,
		IsPartialMock: false,
		Pue:           pue,
		TotalPowerKw:  totalPower,
	})
}

// DcRackData 机柜概览行
type DcRackData struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	AlertCount int    `json:"alert_count"`
}

// ───────────────────────────────────────────────
// 设备分布 / 未分配
// ───────────────────────────────────────────────

func handleDevices(w http.ResponseWriter, r *http.Request) {
	groups, err := repos.DcSlots.DeviceGroups()
	if err != nil {
		httpx.RespondError(w, "获取设备分布失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, map[string]interface{}{"groups": groups})
}

func handleUnallocated(w http.ResponseWriter, r *http.Request) {
	devices, err := repos.DcSlots.UnallocatedDevices()
	if err != nil {
		httpx.RespondError(w, "获取未分配设备失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, devices)
}

// ───────────────────────────────────────────────
// 生命周期
// ───────────────────────────────────────────────

func handleLifecycleList(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 500)
	action := r.URL.Query().Get("action")
	records, err := repos.DcLifecycles.List(limit, action)
	if err != nil {
		httpx.RespondError(w, "获取生命周期记录失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, records)
}

// ───────────────────────────────────────────────
// 导出 / 导入
// ───────────────────────────────────────────────

// DcExportData 导出数据结构（前端 ExportData 对齐）
type DcExportData struct {
	Summary         map[string]int           `json:"summary"`
	Rooms           []repos.DcRoom           `json:"rooms"`
	Racks           []repos.DcRack           `json:"racks"`
	Slots           []DcExportSlot           `json:"slots"`
	Pdus            []repos.DcPdu            `json:"pdus"`
	Manufacturers   []repos.DcManufacturer   `json:"manufacturers"`
	DeviceTypes     []repos.DcDeviceType     `json:"device_types"`
	PowerPanels     []repos.DcPowerPanel     `json:"power_panels"`
	PowerFeeds      []repos.DcPowerFeed      `json:"power_feeds"`
	Cables          []repos.DcCable          `json:"cables"`
	Lifecycles      []repos.DcLifecycleRecord `json:"lifecycles"`
}

// DcExportSlot 导出用机位（展开设备名）
type DcExportSlot struct {
	ID           string  `json:"id"`
	RackID       string  `json:"rack_id"`
	DeviceID     string  `json:"device_id"`
	DeviceType   string  `json:"device_type"`
	DeviceName   string  `json:"device_name"`
	StartU       int     `json:"start_u"`
	EndU         int     `json:"end_u"`
	PositionFace string  `json:"position_face"`
	Notes        *string `json:"notes,omitempty"`
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	rooms, _ := repos.DcRooms.List()
	racks, _ := repos.DcRacks.List()
	pdus, _ := repos.DcPdus.List()
	manufacturers, _ := repos.DcManufacturers.List()
	deviceTypes, _ := repos.DcDeviceTypes.List()
	powerPanels, _ := repos.DcPowerPanels.List()
	powerFeeds, _ := repos.DcPowerFeeds.List()
	cables, _ := repos.DcCables.List()
	lifecycles, _ := repos.DcLifecycles.List(10000, "")

// 机位展开（带设备名）
	var slots []DcExportSlot
	slotRows, err := appdb.DB().Query(`
		SELECT s.id, s.rack_id, s.device_id, s.device_type, s.start_u, s.end_u, s.position_face, s.notes,
		       COALESCE(srv.name, nd.name, '') AS device_name,
		       COALESCE(srv.ip_address, nd.ip_address, '') AS ip_address
		FROM dc_rack_slots s
		LEFT JOIN servers srv ON s.device_type = 'server' AND srv.id = s.device_id
		LEFT JOIN network_devices nd ON s.device_type = 'network_device' AND nd.id = s.device_id`)
	if err == nil {
		defer slotRows.Close()
		for slotRows.Next() {
			var s DcExportSlot
			var notes, deviceName, ipAddr sql.NullString
			if err := slotRows.Scan(&s.ID, &s.RackID, &s.DeviceID, &s.DeviceType, &s.StartU, &s.EndU, &s.PositionFace, &notes, &deviceName, &ipAddr); err != nil {
				continue
			}
			s.DeviceName = deviceName.String
			s.Notes = nullStrPtr(notes)
			slots = append(slots, s)
		}
	}
	if slots == nil {
		slots = []DcExportSlot{}
	}

	data := DcExportData{
		Summary: map[string]int{
			"rooms":   len(rooms),
			"racks":   len(racks),
			"devices": len(slots),
		},
		Rooms:         rooms,
		Racks:         racks,
		Slots:         slots,
		Pdus:          pdus,
		Manufacturers: manufacturers,
		DeviceTypes:   deviceTypes,
		PowerPanels:   powerPanels,
		PowerFeeds:    powerFeeds,
		Cables:        cables,
		Lifecycles:    lifecycles,
	}

	// 支持 Blob 下载（前端 exportBlob 用 responseType:blob）
	accept := r.Header.Get("Accept")
	if accept == "application/octet-stream" || accept == "*/*" && r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="dc_export.json"`)
		_ = json.NewEncoder(w).Encode(data)
		return
	}
	httpx.Respond(w, data)
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data json.RawMessage `json:"data"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	var data struct {
		Rooms []repos.DcRoomCreateInput `json:"rooms"`
		Racks []struct {
			Name    string `json:"name"`
			RoomID  string `json:"room_id"`
			RoomName string `json:"room_name"`
			TotalU  int    `json:"total_u"`
			RowNumber *int `json:"row_number"`
		} `json:"racks"`
	}
	if len(req.Data) > 0 {
		if err := json.Unmarshal(req.Data, &data); err != nil {
			httpx.RespondError(w, "导入数据格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	roomIDMap := map[string]string{}
	createdRooms, createdRacks := 0, 0

	// 先按 name 匹配已有机房，否则创建
	for _, room := range data.Rooms {
		if room.Name == "" {
			continue
		}
		var existingID string
		err := appdb.DB().QueryRow(`SELECT id FROM dc_rooms WHERE name = ?`, room.Name).Scan(&existingID)
		if err == nil {
			roomIDMap[room.Name] = existingID
			continue
		}
		created, err := repos.DcRooms.Create(room)
		if err != nil {
			continue
		}
		roomIDMap[room.Name] = created.ID
		createdRooms++
	}

	for _, rack := range data.Racks {
		if rack.Name == "" {
			continue
		}
		roomID := rack.RoomID
		if roomID == "" && rack.RoomName != "" {
			roomID = roomIDMap[rack.RoomName]
		}
		if roomID == "" {
			continue
		}
		_, err := repos.DcRacks.Create(repos.DcRackCreateInput{
			Name:      rack.Name,
			RoomID:    roomID,
			TotalU:    rack.TotalU,
			RowNumber: intPtr(rack.RowNumber),
		})
		if err != nil {
			continue
		}
		createdRacks++
	}

	httpx.RespondMessage(w, "导入成功")
}

// intPtr 返回 int 指针
func intPtr(v *int) *int {
	if v == nil {
		return nil
	}
	return v
}

// nullStrPtr sql.NullString 转 *string
func nullStrPtr(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}