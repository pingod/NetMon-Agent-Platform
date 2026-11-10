// 厂商适配器 — 工厂注册表与标准巡检维度，等价于 TS vendorAdapter/index.ts
package vendoradapter

import (
	"itops-backend/internal/logger"
)

// adapterFactory 按厂商类型创建适配器的工厂函数表
var adapterFactory = map[VendorType]func() VendorAdapter{
	VendorHuawei:    newHuaweiAdapter,
	VendorCisco:     newCiscoAdapter,
	VendorH3c:       newH3cAdapter,
	VendorRuijie:    newRuijieAdapter,
	VendorRuijieEG:  newRuijieEGAdapter,
	VendorZte:       newZteAdapter,
	VendorFortinet:  newFortinetAdapter,
	VendorPaloAlto:  newPaloAltoAdapter,
	VendorJuniper:   newJuniperAdapter,
	VendorArista:    newAristaAdapter,
	VendorHpe:       newHpeAdapter,
	VendorMikrotik:  newMikrotikAdapter,
	VendorUbiquiti:  newUbiquitiAdapter,
	VendorDell:      newDellAdapter,
	VendorTplink:    newTplinkAdapter,
	VendorF5:        newF5Adapter,
	VendorMaipu:     newMaipuAdapter,
	VendorSangfor:   newSangforAdapter,
	VendorOther:     newOtherAdapter,
}

// CreateVendorAdapter 按厂商名称创建适配器实例
// 未知厂商回退到通用适配器（other），不再回退到华为
func CreateVendorAdapter(vendor VendorType) VendorAdapter {
	factory, ok := adapterFactory[vendor]
	if !ok {
		logger.Warnf("Unknown vendor: %s, falling back to generic (other) adapter", vendor)
		return newOtherAdapter()
	}
	return factory()
}

// StandardInspectionTypes 所有标准巡检维度（全量），与 TS 端保持一致
var StandardInspectionTypes = []InspectionType{
	InspectCPU,
	InspectMemory,
	InspectInterface,
	InspectVersion,
	InspectRoutes,
	InspectLog,
	InspectEnvironment,
	InspectPower,
	InspectFan,
	InspectSTP,
	InspectVLAN,
	InspectARP,
	InspectMAC,
	// 扩展维度
	InspectOptic,
	InspectNeighbor,
	InspectSecurityPolicy,
	InspectNAT,
	InspectSession,
	InspectVPN,
	InspectWLAN,
	InspectPool,
	InspectDNS,
	InspectBGP,
	InspectOSPF,
	InspectNTP,
	InspectLicense,
	InspectConfigChecksum,
}
