// NetBox 资源处理函数：制造商 / 设备型号 / 配电柜 / 供电线路 / 线缆
package dcroutes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 制造商
// ───────────────────────────────────────────────

func handleManufacturerList(w http.ResponseWriter, r *http.Request) {
	items, err := repos.DcManufacturers.List()
	if err != nil {
		httpx.RespondError(w, "获取制造商列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleManufacturerCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpx.RespondError(w, "制造商名称不能为空", http.StatusBadRequest)
		return
	}
	item, err := repos.DcManufacturers.Create(req.Name, req.Slug, ptrStr(req.Description))
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleManufacturerUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Slug        *string `json:"slug"`
		Description *string `json:"description"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	item, err := repos.DcManufacturers.Update(chi.URLParam(r, "id"), req.Name, req.Slug, req.Description)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleManufacturerDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcManufacturers.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "制造商已删除")
}

// ───────────────────────────────────────────────
// 设备型号
// ───────────────────────────────────────────────

func handleDeviceTypeList(w http.ResponseWriter, r *http.Request) {
	items, err := repos.DcDeviceTypes.List()
	if err != nil {
		httpx.RespondError(w, "获取设备型号列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleDeviceTypeCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManufacturerID string   `json:"manufacturer_id"`
		Model          string   `json:"model"`
		Slug           string   `json:"slug"`
		Description    *string  `json:"description"`
		UHeight        *float64 `json:"u_height"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.ManufacturerID == "" || req.Model == "" {
		httpx.RespondError(w, "制造商和设备型号不能为空", http.StatusBadRequest)
		return
	}
	item, err := repos.DcDeviceTypes.CreateByID(req.ManufacturerID, req.Model, req.Slug, ptrStr(req.Description), req.UHeight)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleDeviceTypeUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManufacturerID *string  `json:"manufacturer_id"`
		Model          *string  `json:"model"`
		Slug           *string  `json:"slug"`
		Description    *string  `json:"description"`
		UHeight        *float64 `json:"u_height"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	item, err := repos.DcDeviceTypes.Update(chi.URLParam(r, "id"), req.ManufacturerID, req.Model, req.Slug, req.Description, req.UHeight)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleDeviceTypeDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcDeviceTypes.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "设备型号已删除")
}

// ───────────────────────────────────────────────
// 配电柜
// ───────────────────────────────────────────────

func handlePowerPanelList(w http.ResponseWriter, r *http.Request) {
	items, err := repos.DcPowerPanels.List()
	if err != nil {
		httpx.RespondError(w, "获取配电柜列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handlePowerPanelCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RoomID      string   `json:"room_id"`
		Name        string   `json:"name"`
		PanelType   string   `json:"panel_type"`
		Description *string  `json:"description"`
		Voltage     *float64 `json:"voltage"`
		Amperage    *float64 `json:"amperage"`
		PhaseCount  *int     `json:"phase_count"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.RoomID == "" || req.Name == "" {
		httpx.RespondError(w, "所属机房和配电柜名称不能为空", http.StatusBadRequest)
		return
	}
	item, err := repos.DcPowerPanels.CreateByID(req.RoomID, req.Name, req.PanelType, ptrStr(req.Description), req.Voltage, req.Amperage, req.PhaseCount)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handlePowerPanelUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RoomID      *string  `json:"room_id"`
		Name        *string  `json:"name"`
		PanelType   *string  `json:"panel_type"`
		Description *string  `json:"description"`
		Voltage     *float64 `json:"voltage"`
		Amperage    *float64 `json:"amperage"`
		PhaseCount  *int     `json:"phase_count"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	item, err := repos.DcPowerPanels.Update(chi.URLParam(r, "id"), req.RoomID, req.Name, req.PanelType, req.Description, req.Voltage, req.Amperage, req.PhaseCount)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handlePowerPanelDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcPowerPanels.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "配电柜已删除")
}

// ───────────────────────────────────────────────
// 供电线路
// ───────────────────────────────────────────────

func handlePowerFeedList(w http.ResponseWriter, r *http.Request) {
	items, err := repos.DcPowerFeeds.List()
	if err != nil {
		httpx.RespondError(w, "获取供电线路列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handlePowerFeedCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PanelID   string   `json:"panel_id"`
		Name      string   `json:"name"`
		RackID    *string  `json:"rack_id"`
		Voltage   *float64 `json:"voltage"`
		Amperage  *float64 `json:"amperage"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.PanelID == "" || req.Name == "" {
		httpx.RespondError(w, "所属配电柜和线路名称不能为空", http.StatusBadRequest)
		return
	}
	item, err := repos.DcPowerFeeds.CreateByID(req.PanelID, req.Name, req.RackID, req.Voltage, req.Amperage)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handlePowerFeedUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PanelID  *string  `json:"panel_id"`
		RackID   *string  `json:"rack_id"`
		Name     *string  `json:"name"`
		Voltage  *float64 `json:"voltage"`
		Amperage *float64 `json:"amperage"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	item, err := repos.DcPowerFeeds.Update(chi.URLParam(r, "id"), req.PanelID, req.RackID, req.Name, req.Voltage, req.Amperage)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handlePowerFeedDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcPowerFeeds.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "供电线路已删除")
}

// ───────────────────────────────────────────────
// 线缆
// ───────────────────────────────────────────────

func handleCableList(w http.ResponseWriter, r *http.Request) {
	items, err := repos.DcCables.List()
	if err != nil {
		httpx.RespondError(w, "获取线缆列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, items)
}

func handleCableCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		CableType    *string  `json:"cable_type"`
		LengthM      *float64 `json:"length_m"`
		ADeviceID    string   `json:"a_device_id"`
		ADeviceType  string   `json:"a_device_type"`
		BDeviceID    string   `json:"b_device_id"`
		BDeviceType  string   `json:"b_device_type"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	if req.ADeviceID == "" || req.BDeviceID == "" {
		httpx.RespondError(w, "线缆两端设备不能为空", http.StatusBadRequest)
		return
	}
	item, err := repos.DcCables.CreateByID(req.Name, req.CableType, req.LengthM, req.ADeviceID, req.ADeviceType, req.BDeviceID, req.BDeviceType)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleCableUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         *string  `json:"name"`
		CableType    *string  `json:"cable_type"`
		CableColor   *string  `json:"cable_color"`
		LengthM      *float64 `json:"length_m"`
		ADeviceID    *string  `json:"a_device_id"`
		ADeviceType  *string  `json:"a_device_type"`
		BDeviceID    *string  `json:"b_device_id"`
		BDeviceType  *string  `json:"b_device_type"`
	}
	if !parseJSON(w, r, &req) {
		return
	}
	item, err := repos.DcCables.Update(chi.URLParam(r, "id"), req.Name, req.CableType, req.CableColor, req.LengthM,
		req.ADeviceID, req.ADeviceType, req.BDeviceID, req.BDeviceType)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, item)
}

func handleCableDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcCables.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "线缆已删除")
}

// ptrStr 本地工具：*string → string
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}