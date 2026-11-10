// 将巡检采集到的接口/系统指标，按 snmp_metric_definitions 命名体系派生标签后写入 snmp_metric_samples。
//
// 设计：复用 InspectDevice 已经 WALK 得到的接口数据（InterfaceInfo），避免重复 SNMP 请求。
//   - 每个 IF-MIB 表列指标 → 一条样本，tag 含 ifIndex=索引、ifDescr/ifName=可读名
//   - 系统标量 → 一条样本，tag 仅含 device 自身
// 这样首页大屏 / 面板编辑器即可用 Grafana 式查询统一消费所有 SNMP 指标。
package services

import (
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/repos"
)

// WriteSnmpMetricSamplesFromInspect 根据巡检结果派生并写入命名指标样本。
// 非阻塞、非致命：任何错误仅记录日志，不影响主巡检流程。
func WriteSnmpMetricSamplesFromInspect(deviceID string, sysInfo *SystemInfo, interfaces []InterfaceInfo) {
	defer func() {
		if r := recover(); r != nil {
			SnmpMetricSeedLog("write snmp_metric_samples panic recovered: %v", r)
		}
	}()

	defs, err := repos.SnmpMetricDefs.List("IF-MIB")
	if err != nil {
		SnmpMetricSeedLog("load IF-MIB definitions failed: %v", err)
		return
	}
	defByName := map[string]repos.SnmpMetricDefinition{}
	for _, d := range defs {
		defByName[d.MetricName] = d
	}
	if len(defByName) == 0 {
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	var samples []repos.SnmpMetricSample

	// 系统标量
	if sysInfo != nil {
		if d, ok := defByName["sysUptime"]; ok {
			if v, e := parseFloatValue(sysInfo.SysUptime); e == nil {
				samples = append(samples, buildSample(d, deviceID, map[string]string{}, v, now))
			}
		}
	}

	// 接口表列
	for _, iface := range interfaces {
		idxStr := fmt.Sprintf("%d", iface.Index)
		tags := map[string]string{
			"ifIndex": idxStr,
		}
		if iface.Descr != "" {
			tags["ifDescr"] = iface.Descr
		}
		if iface.Name != "" {
			tags["ifName"] = iface.Name
		}
		if iface.Alias != "" {
			tags["ifAlias"] = iface.Alias
		}

		emit := func(name string, val interface{}) {
			d, ok := defByName[name]
			if !ok {
				return
			}
			v, e := parseFloatValue(val)
			if e != nil {
				return
			}
			samples = append(samples, buildSample(d, deviceID, tags, v, now))
		}

		emit("ifDescr", float64(iface.Index))        // 描述列（以索引占位，便于标签对齐）
		emit("ifType", float64(iface.Type))
		emit("ifMtu", float64(iface.MTU))
		emit("ifSpeed", float64(iface.Speed))
		emit("ifAdminStatus", adminStatusToFloat(iface.AdminStatus))
		emit("ifOperStatus", operStatusToFloat(iface.OperStatus))
		emit("ifInOctets", float64(iface.InOctets))
		emit("ifInUcastPkts", float64(iface.InUcastPkts))
		emit("ifInErrors", float64(iface.InErrors))
		emit("ifOutOctets", float64(iface.OutOctets))
		emit("ifOutUcastPkts", float64(iface.OutUcastPkts))
		emit("ifOutErrors", float64(iface.OutErrors))
		emit("ifHighSpeed", float64(iface.Speed/1e6)) // ifHighSpeed 单位 Mbps
		emit("ifHCInOctets", float64(iface.InOctets))
		emit("ifHCOutOctets", float64(iface.OutOctets))
		emit("ifName", float64(iface.Index))
		emit("ifAlias", float64(iface.Index))
		emit("ifIndex", float64(iface.Index))
	}

	if len(samples) == 0 {
		return
	}
	if err := repos.SnmpMetricSamples.InsertBatch(samples); err != nil {
		SnmpMetricSeedLog("insert snmp_metric_samples failed: %v", err)
		return
	}
	SnmpMetricSeedLog("wrote %d snmp_metric_samples for device %s", len(samples), deviceID)
}

// buildSample 构造一条样本（含 definition_id 与标签 JSON）
func buildSample(def repos.SnmpMetricDefinition, deviceID string, tags map[string]string, value float64, ts string) repos.SnmpMetricSample {
	tagsJSON, _ := json.Marshal(tags)
	return repos.SnmpMetricSample{
		DefinitionID: def.ID,
		MetricName:   def.MetricName,
		DeviceID:     deviceID,
		TagsJSON:     string(tagsJSON),
		Value:        value,
		CollectedAt:  ts,
	}
}

func adminStatusToFloat(s string) float64 {
	if s == "up" {
		return 1
	}
	if s == "down" {
		return 2
	}
	return 0
}

func operStatusToFloat(s string) float64 {
	if s == "up" {
		return 1
	}
	if s == "down" {
		return 2
	}
	return 0
}

// parseFloatValue 将多种数值类型安全地转为 float64
func parseFloatValue(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%f", &f); err != nil {
			return 0, err
		}
		return f, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}
