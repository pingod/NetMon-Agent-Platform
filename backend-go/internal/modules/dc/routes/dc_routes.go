// 数据中心（DC）模块路由 — 等价于前端 modules/dc/api.ts
// 覆盖：机房 / 机柜 / 机位 / PDU / 概览 / 设备分布 / 生命周期 / 导出导入 /
//
//	制造商 / 设备型号 / 配电柜 / 供电线路 / 线缆
package dcroutes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
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
	r.With(middleware.RequireRole("admin", "operator")).Post("/import", handleImport)

	// ── 机房 ──
	r.Route("/rooms", func(r chi.Router) {
		r.Get("/", handleRoomList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleRoomCreate)
		r.Get("/{id}", handleRoomGet)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleRoomUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleRoomDelete)
	})

	// ── 机柜 ──
	r.Route("/racks", func(r chi.Router) {
		r.Get("/", handleRackList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleRackCreate)
		r.Get("/{id}", handleRackGet)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleRackUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleRackDelete)
	})

	// ── 机位 ──
	r.Get("/batch-slots", handleSlotListAll)
	r.Route("/slots", func(r chi.Router) {
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleSlotCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleSlotUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleSlotDelete)
	})
	r.Get("/slots/{rackId}", handleSlotListByRack)

	// ── PDU ──
	r.Route("/pdus", func(r chi.Router) {
		r.Get("/", handlePduList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handlePduCreate)
		r.Get("/{id}", handlePduGet)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handlePduUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handlePduDelete)
	})

	// ── 制造商 ──
	r.Route("/manufacturers", func(r chi.Router) {
		r.Get("/", handleManufacturerList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleManufacturerCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleManufacturerUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleManufacturerDelete)
	})

	// ── 设备型号 ──
	r.Route("/device-types", func(r chi.Router) {
		r.Get("/", handleDeviceTypeList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleDeviceTypeCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleDeviceTypeUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleDeviceTypeDelete)
	})

	// ── 配电柜 ──
	r.Route("/power-panels", func(r chi.Router) {
		r.Get("/", handlePowerPanelList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handlePowerPanelCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handlePowerPanelUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handlePowerPanelDelete)
	})

	// ── 供电线路 ──
	r.Route("/power-feeds", func(r chi.Router) {
		r.Get("/", handlePowerFeedList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handlePowerFeedCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handlePowerFeedUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handlePowerFeedDelete)
	})

	// ── 线缆 ──
	r.Route("/cables", func(r chi.Router) {
		r.Get("/", handleCableList)
		r.With(middleware.RequireRole("admin", "operator")).Post("/", handleCableCreate)
		r.With(middleware.RequireRole("admin", "operator")).Put("/{id}", handleCableUpdate)
		r.With(middleware.RequireRole("admin", "operator")).Delete("/{id}", handleCableDelete)
	})

	return r
}

// 兜底：任何未知 DC 端点返回 404（避免误入 SPA 回退）
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.RespondError(w, "DC 端点不存在", http.StatusNotFound)
}
