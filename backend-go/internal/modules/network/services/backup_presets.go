// 备份模版管理 — 预置模版数据与种子函数
package services

import (
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

// PresetBackupTemplate 预置配置备份模版定义
type PresetBackupTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
}

// PresetBackupTemplates 返回主流厂商的内置标准备份模版
func PresetBackupTemplates() []PresetBackupTemplate {
	return []PresetBackupTemplate{
		{
			ID:          "tpl-builtin-huawei",
			Name:        "华为设备配置备份模板",
			Vendor:      "huawei",
			Description: "适用于华为 (Huawei) 交换机、路由器及安全网关的标准运行配置导出",
			Commands: []string{
				"display current-configuration",
			},
		},
		{
			ID:          "tpl-builtin-h3c",
			Name:        "华三设备配置备份模板",
			Vendor:      "h3c",
			Description: "适用于华三 (H3C) Comware V5/V7 交换机及路由器的标准运行配置导出",
			Commands: []string{
				"display current-configuration",
			},
		},
		{
			ID:          "tpl-builtin-cisco",
			Name:        "思科设备配置备份模板",
			Vendor:      "cisco",
			Description: "适用于思科 (Cisco) IOS/IOS-XE/NX-OS 设备的 running-config 标准配置导出",
			Commands: []string{
				"show running-config",
			},
		},
		{
			ID:          "tpl-builtin-zte",
			Name:        "中兴设备配置备份模板",
			Vendor:      "zte",
			Description: "适用于中兴 (ZTE) 交换机、路由器及传输网元设备的运行配置导出",
			Commands: []string{
				"show running-config",
			},
		},
		{
			ID:          "tpl-builtin-ruijie",
			Name:        "锐捷设备配置备份模板",
			Vendor:      "ruijie",
			Description: "适用于锐捷 (Ruijie) RGOS 交换机、网关与无线 AC 设备的运行配置导出",
			Commands: []string{
				"show running-config",
			},
		},
		{
			ID:          "tpl-builtin-fortinet",
			Name:        "飞塔防火墙配置备份模板",
			Vendor:      "fortinet",
			Description: "适用于 Fortinet FortiGate 安全网关与防火墙的完整配置导出",
			Commands: []string{
				"show full-configuration",
			},
		},
		{
			ID:          "tpl-builtin-juniper",
			Name:        "Juniper 设备配置备份模板",
			Vendor:      "juniper",
			Description: "适用于 Juniper JunOS 设备的 set 格式运行配置导出",
			Commands: []string{
				"show configuration | display set",
			},
		},
		{
			ID:          "tpl-builtin-paloalto",
			Name:        "Palo Alto 防火墙配置备份模板",
			Vendor:      "paloalto",
			Description: "适用于 Palo Alto PAN-OS 新一代防火墙的运行配置导出",
			Commands: []string{
				"show config running",
			},
		},
	}
}

// EnsureBackupPresets 确保预置备份模版存在于数据库中（幂等初始化）
func EnsureBackupPresets() error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("数据库连接未就绪")
	}

	// 检查当前模版总数
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM config_backup_templates").Scan(&count)
	if err != nil {
		return fmt.Errorf("查询配置备份模版失败: %w", err)
	}

	// 如果没有任何模版，全量注入预置模版
	if count == 0 {
		return insertAllBackupPresets()
	}

	// 增量检查：如果预置模版缺失，按 ID 自动补齐
	presets := PresetBackupTemplates()
	inserted := 0
	for _, p := range presets {
		var exists int
		_ = db.QueryRow("SELECT COUNT(*) FROM config_backup_templates WHERE id = ?", p.ID).Scan(&exists)
		if exists == 0 {
			// 也检查是否同名同厂商已存在（避免重复）
			var nameExists int
			_ = db.QueryRow("SELECT COUNT(*) FROM config_backup_templates WHERE name = ? AND vendor = ?", p.Name, p.Vendor).Scan(&nameExists)
			if nameExists == 0 {
				cmdsJSON, _ := json.Marshal(p.Commands)
				_, err := db.Exec(`
					INSERT INTO config_backup_templates (id, name, description, vendor, commands, is_builtin, created_by)
					VALUES (?, ?, ?, ?, ?, 1, 'system')
				`, p.ID, p.Name, p.Description, p.Vendor, string(cmdsJSON))
				if err == nil {
					inserted++
				}
			}
		}
	}

	if inserted > 0 {
		logger.Infof("已自动补充 %d 个主流厂商备份预置模版", inserted)
	}
	return nil
}

// insertAllBackupPresets 全量插入所有预置备份模版
func insertAllBackupPresets() error {
	db := appdb.DB()
	presets := PresetBackupTemplates()
	for _, p := range presets {
		cmdsJSON, _ := json.Marshal(p.Commands)
		_, err := db.Exec(`
			INSERT INTO config_backup_templates (id, name, description, vendor, commands, is_builtin, created_by)
			VALUES (?, ?, ?, ?, ?, 1, 'system')
		`, p.ID, p.Name, p.Description, p.Vendor, string(cmdsJSON))
		if err != nil {
			logger.Warnf("插入预置模版 %s 失败: %v", p.Name, err)
		}
	}
	logger.Infof("已成功初始化 %d 个主流厂商配置备份模版（华为/华三/思科/中兴/锐捷/飞塔等）", len(presets))
	return nil
}

// RestoreBackupPresets 恢复所有默认预置模版
func RestoreBackupPresets() ([]*repos.ConfigBackupTemplate, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("数据库连接未就绪")
	}

	presets := PresetBackupTemplates()
	for _, p := range presets {
		cmdsJSON, _ := json.Marshal(p.Commands)
		// 使用 INSERT OR REPLACE 或先查后插更新
		var exists int
		_ = db.QueryRow("SELECT COUNT(*) FROM config_backup_templates WHERE id = ?", p.ID).Scan(&exists)
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		if exists > 0 {
			_, _ = db.Exec(`
				UPDATE config_backup_templates
				SET name = ?, description = ?, vendor = ?, commands = ?, is_builtin = 1, updated_at = ?
				WHERE id = ?
			`, p.Name, p.Description, p.Vendor, string(cmdsJSON), nowStr, p.ID)
		} else {
			_, _ = db.Exec(`
				INSERT INTO config_backup_templates (id, name, description, vendor, commands, is_builtin, created_by, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, 1, 'system', ?, ?)
			`, p.ID, p.Name, p.Description, p.Vendor, string(cmdsJSON), nowStr, nowStr)
		}
	}

	return repos.ConfigBackupTemplates.List()
}
