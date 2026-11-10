// 厂商适配器 — 默认配置备份命令
// 当用户未指定备份模板时，系统自动使用对应厂商的默认备份命令
package vendoradapter

// DefaultBackupCommands 返回指定厂商的默认配置备份命令
// 返回的命令列表将依次在 Shell 或 Exec 模式下执行
func DefaultBackupCommands(vendor VendorType) []string {
	switch vendor {
	case VendorHuawei:
		return []string{"display current-configuration"}

	case VendorCisco:
		return []string{"show running-config"}

	case VendorH3c:
		return []string{"display current-configuration"}

	case VendorRuijie, VendorRuijieEG:
		return []string{"show running-config"}

	case VendorZte:
		return []string{"show running-config"}

	case VendorFortinet:
		// FortiGate 需要执行多条命令来导出完整配置
		return []string{"show full-configuration"}

	case VendorPaloAlto:
		return []string{"show config running"}

	case VendorJuniper:
		// JunOS: show configuration | display set — 显示为 set 命令格式
		return []string{"show configuration | display set"}

	case VendorArista:
		return []string{"show running-config"}

	case VendorHpe:
		// HPE Comware: 与 H3C 语法一致
		return []string{"display current-configuration"}

	case VendorMikrotik:
		// RouterOS: export 命令导出配置
		return []string{"/export"}

	case VendorUbiquiti:
		// EdgeMAX/UniFi: show configuration
		return []string{"show configuration"}

	case VendorDell:
		// Dell Force10 / OS9: show running-config
		return []string{"show running-config"}

	case VendorTplink:
		return []string{"show running-config"}

	case VendorF5:
		// F5 BIG-IP: tmsh show running-config
		return []string{"tmsh show running-config"}

	case VendorOther:
		// 通用：依次尝试最常见的命令（第一个成功的会返回有效输出）
		return []string{"show running-config", "display current-configuration"}

	default:
		// 默认：尝试两种最常见的命令
		return []string{"show running-config", "display current-configuration"}
	}
}

// DefaultBackupCommandNames 返回指定厂商的默认备份命令名称（用于模板名称）
func DefaultBackupCommandNames(vendor VendorType) string {
	switch vendor {
	case VendorHuawei, VendorH3c, VendorHpe:
		return "当前运行配置"
	case VendorCisco, VendorRuijie, VendorRuijieEG, VendorZte, VendorArista, VendorDell, VendorTplink:
		return "Running Configuration"
	case VendorFortinet:
		return "Full Configuration"
	case VendorPaloAlto:
		return "Running Config"
	case VendorJuniper:
		return "Configuration (set format)"
	case VendorMikrotik:
		return "RouterOS Export"
	case VendorUbiquiti:
		return "Configuration"
	case VendorF5:
		return "BIG-IP Running Config"
	default:
		return "Running Configuration"
	}
}
