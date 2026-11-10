// 机房 / 机柜 / 机位 / PDU 处理函数
package dcroutes

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// parseJSON 解析请求 JSON body，失败时返回 400
func parseJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpx.RespondError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ───────────────────────────────────────────────
// 机房
// ───────────────────────────────────────────────

func handleRoomList(w http.ResponseWriter, r *http.Request) {
	rooms, err := repos.DcRooms.List()
	if err != nil {
		httpx.RespondError(w, "获取机房列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, rooms)
}

func handleRoomGet(w http.ResponseWriter, r *http.Request) {
	room, err := repos.DcRooms.GetByID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "机房不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, room)
}

func handleRoomCreate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcRoomCreateInput
	if !parseJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpx.RespondError(w, "机房名称不能为空", http.StatusBadRequest)
		return
	}
	room, err := repos.DcRooms.Create(req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, room)
}

func handleRoomUpdate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcRoomUpdateInput
	if !parseJSON(w, r, &req) {
		return
	}
	room, err := repos.DcRooms.Update(chi.URLParam(r, "id"), req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, room)
}

func handleRoomDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcRooms.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "机房已删除")
}

// ───────────────────────────────────────────────
// 机柜
// ───────────────────────────────────────────────

func handleRackList(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room_id")
	if roomID != "" {
		racks, err := repos.DcRacks.ListByRoom(roomID)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, racks)
		return
	}
	racks, err := repos.DcRacks.List()
	if err != nil {
		httpx.RespondError(w, "获取机柜列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, racks)
}

func handleRackGet(w http.ResponseWriter, r *http.Request) {
	rack, err := repos.DcRacks.GetByID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "机柜不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, rack)
}

func handleRackCreate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcRackCreateInput
	if !parseJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpx.RespondError(w, "机柜名称不能为空", http.StatusBadRequest)
		return
	}
	rack, err := repos.DcRacks.Create(req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, rack)
}

func handleRackUpdate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcRackUpdateInput
	if !parseJSON(w, r, &req) {
		return
	}
	rack, err := repos.DcRacks.Update(chi.URLParam(r, "id"), req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, rack)
}

func handleRackDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcRacks.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "机柜已删除")
}

// ───────────────────────────────────────────────
// 机位
// ───────────────────────────────────────────────

func handleSlotListByRack(w http.ResponseWriter, r *http.Request) {
	slots, err := repos.DcSlots.ListByRack(chi.URLParam(r, "rackId"))
	if err != nil {
		httpx.RespondError(w, "获取机位列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, slots)
}

func handleSlotCreate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcSlotCreateInput
	if !parseJSON(w, r, &req) {
		return
	}
	if req.RackID == "" {
		httpx.RespondError(w, "机柜不能为空", http.StatusBadRequest)
		return
	}
	if req.DeviceID == "" {
		req.DeviceID = uuid.NewString()
	}
	if req.StartU <= 0 || req.EndU < req.StartU {
		httpx.RespondError(w, "U位范围无效", http.StatusBadRequest)
		return
	}
	slot, err := repos.DcSlots.Create(req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, slot)
}

func handleSlotUpdate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcSlotUpdateInput
	if !parseJSON(w, r, &req) {
		return
	}
	slot, err := repos.DcSlots.Update(chi.URLParam(r, "id"), req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, slot)
}

func handleSlotDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcSlots.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "机位已删除")
}

// ───────────────────────────────────────────────
// PDU
// ───────────────────────────────────────────────

func handlePduList(w http.ResponseWriter, r *http.Request) {
	pdus, err := repos.DcPdus.List()
	if err != nil {
		httpx.RespondError(w, "获取PDU列表失败", http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, pdus)
}

func handlePduGet(w http.ResponseWriter, r *http.Request) {
	pdu, err := repos.DcPdus.GetByID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "PDU不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, pdu)
}

func handlePduCreate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcPduCreateInput
	if !parseJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpx.RespondError(w, "PDU名称不能为空", http.StatusBadRequest)
		return
	}
	pdu, err := repos.DcPdus.Create(req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, pdu)
}

func handlePduUpdate(w http.ResponseWriter, r *http.Request) {
	var req repos.DcPduUpdateInput
	if !parseJSON(w, r, &req) {
		return
	}
	pdu, err := repos.DcPdus.Update(chi.URLParam(r, "id"), req)
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.Respond(w, pdu)
}

func handlePduDelete(w http.ResponseWriter, r *http.Request) {
	if err := repos.DcPdus.Delete(chi.URLParam(r, "id")); err != nil {
		httpx.RespondError(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.RespondMessage(w, "PDU已删除")
}

// queryInt 读取查询参数整数（带默认值）
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// isNoRows 判断是否 sql.ErrNoRows
func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}