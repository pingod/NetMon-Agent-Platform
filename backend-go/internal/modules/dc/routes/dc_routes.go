// 数据中心（DC）模块路由 — 等价于前端 modules/dc/api.ts
// 覆盖：机房 / 机柜 / 机位 / PDU / 概览 / 设备分布 / 生命周期 / 导出导入 /
//       制造商 / 设备型号 / 配电柜 / 供电线路 / 线缆
package dcroutes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
)

// Routes DC 模块路由入口
func Routes() chi.Router {
	r := chi.NewRouter()

	// ── 概览 / 设备 / 生命周期 / 导出 / 导入 ──
	r.Get("/overview", handleOverview)
	r.Get("/devices", handleDevices)
	r.Get("/devices/unallocated", handleUnallocated)
	r.Get("/lifecycle", handleLifecycleList)
	r.Get("/export", handleExport)
	r.Post("/import", handleImport)

	// ── 机房 ──
	r.Route("/rooms", func(r chi.Router) {
		r.Get("/", handleRoomList)
		r.Post("/", handleRoomCreate)
		r.Get("/{id}", handleRoomGet)
		r.Put("/{id}", handleRoomUpdate)
		r.Delete("/{id}", handleRoomDelete)
	})

	// ── 机柜 ──
	r.Route("/racks", func(r chi.Router) {
		r.Get("/", handleRackList)
		r.Post("/", handleRackCreate)
		r.Get("/{id}", handleRackGet)
		r.Put("/{id}", handleRackUpdate)
		r.Delete("/{id}", handleRackDelete)
	})

	// ── 机位 ──
	r.Route("/slots", func(r chi.Router) {
		r.Post("/", handleSlotCreate)
		r.Put("/{id}", handleSlotUpdate)
		r.Delete("/{id}", handleSlotDelete)
	})
	r.Get("/slots/{rackId}", handleSlotListByRack)

	// ── PDU ──
	r.Route("/pdus", func(r chi.Router) {
		r.Get("/", handlePduList)
		r.Post("/", handlePduCreate)
		r.Get("/{id}", handlePduGet)
		r.Put("/{id}", handlePduUpdate)
		r.Delete("/{id}", handlePduDelete)
	})

	// ── 制造商 ──
	r.Route("/manufacturers", func(r chi.Router) {
		r.Get("/", handleManufacturerList)
		r.Post("/", handleManufacturerCreate)
		r.Put("/{id}", handleManufacturerUpdate)
		r.Delete("/{id}", handleManufacturerDelete)
	})

	// ── 设备型号 ──
	r.Route("/device-types", func(r chi.Router) {
		r.Get("/", handleDeviceTypeList)
		r.Post("/", handleDeviceTypeCreate)
		r.Put("/{id}", handleDeviceTypeUpdate)
		r.Delete("/{id}", handleDeviceTypeDelete)
	})

	// ── 配电柜 ──
	r.Route("/power-panels", func(r chi.Router) {
		r.Get("/", handlePowerPanelList)
		r.Post("/", handlePowerPanelCreate)
		r.Put("/{id}", handlePowerPanelUpdate)
		r.Delete("/{id}", handlePowerPanelDelete)
	})

	// ── 供电线路 ──
	r.Route("/power-feeds", func(r chi.Router) {
		r.Get("/", handlePowerFeedList)
		r.Post("/", handlePowerFeedCreate)
		r.Put("/{id}", handlePowerFeedUpdate)
		r.Delete("/{id}", handlePowerFeedDelete)
	})

	// ── 线缆 ──
	r.Route("/cables", func(r chi.Router) {
		r.Get("/", handleCableList)
		r.Post("/", handleCableCreate)
		r.Put("/{id}", handleCableUpdate)
		r.Delete("/{id}", handleCableDelete)
	})

	return r
}

// 兜底：任何未知 DC 端点返回 404（避免误入 SPA 回退）
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.RespondError(w, "DC 端点不存在", http.StatusNotFound)
}