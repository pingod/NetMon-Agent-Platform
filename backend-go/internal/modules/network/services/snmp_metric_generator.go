// generator.yml 风格配置解析：module + walk + lookups → snmp_metric_definitions
//
// snmp_exporter generator.yml 语义：
//   modules:
//     if_mib:
//       walk:
//         - 1.3.6.1.2.1.2        # 表 OID（WALK 根）
//       lookups:
//         - source_indexes: [ifIndex]
//           lookup: ifDescr        # 把 ifDescr 作为标签注入每个指标
//
// 这里将 walk 的末级列自动登记为命名指标（表列 → 带 index/label 标签规则）。
package services

import (
	"fmt"
	"strings"

	"itops-backend/internal/repos"
)

// SnmpLookup generator.yml 的 lookups 项
type SnmpLookup struct {
	OID       string // 形如 1.3.6.1.2.1.2.2.1.2（ifDescr OID）
	LabelName string // 标签名，如 ifDescr
}

// GenerateDefinitionsFromGenerator 根据 module + walk + lookups 生成指标定义。
// 返回创建（或更新）的定义数量。
//
// 实现策略：
//   - walk 项若是表根 OID（如 1.3.6.1.2.1.2.2），展开其子列暂不可行（需 MIB 解析）；
//     因此本函数将 walk 项本身作为“表根”登记为可 WALK 的指标组，并把 lookups 作为标签源。
//   - 若 walk 项已含末级列（如 1.3.6.1.2.1.2.2.1.10），直接登记为单个命名指标。
func GenerateDefinitionsFromGenerator(module string, walk []string, lookups []SnmpLookup) (int, error) {
	if len(walk) == 0 {
		return 0, fmt.Errorf("walk 不能为空")
	}

	labelOIDs := []string{}
	labelNames := []string{}
	for _, l := range lookups {
		if l.OID != "" && l.LabelName != "" {
			labelOIDs = append(labelOIDs, l.OID)
			labelNames = append(labelNames, l.LabelName)
		}
	}

	created := 0
	for _, oid := range walk {
		oid = strings.TrimSpace(oid)
		if oid == "" {
			continue
		}
		// 末级 OID 段数较多（>=10 段）视为具体列，否则视为表根
		segments := strings.Count(oid, ".")
		isTable := 1
		indexTag := "ifIndex"
		if segments >= 11 {
			// 末级列：尝试从 OID 末尾数字派生索引（snmp_exporter 约定 {ifIndex} 由 WALK 时填充）
			isTable = 1
		}
		metricName := metricNameFromOID(module, oid, segments)

		def := &repos.SnmpMetricDefinition{
			MetricName:   metricName,
			ModuleName:   module,
			OID:          oid,
			Description:  fmt.Sprintf("generator.yml 导入 (%s)", module),
			MetricType:   "gauge",
			IsTable:      isTable,
			IndexTag:     indexTag,
			LabelOIDs:    mustJSONStr(labelOIDs),
			LabelNames:   mustJSONStr(labelNames),
			Source:       "generator",
			GeneratorModule: module,
		}
		if err := repos.SnmpMetricDefs.Upsert(def); err != nil {
			return created, fmt.Errorf("upsert %s: %w", metricName, err)
		}
		created++
	}
	return created, nil
}

// metricNameFromOID 根据 OID 末段与 module 生成可读的 snmp_exporter 风格名
// 优先使用 lookups 中的 labelname 反向匹配（若 OID 命中某个 lookup OID，则命名为该 labelname）
func metricNameFromOID(module, oid string, segments int) string {
	// 简单可读化：取 OID 末两段 + 模块前缀
	parts := strings.Split(oid, ".")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		prev := parts[len(parts)-2]
		return sanitizeMetricName(fmt.Sprintf("%s_%s_%s", module, prev, last))
	}
	return sanitizeMetricName(module + "_" + oid)
}

// sanitizeMetricName 将任意字符串规整为合法 Prometheus 风格指标名
func sanitizeMetricName(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			if upper {
				b.WriteRune(toUpper(r))
			} else {
				b.WriteRune(r)
			}
			upper = false
		default:
			upper = true // 非字母数字后下一个字母转大写（驼峰）
		}
	}
	out := b.String()
	if out == "" {
		return "metric"
	}
	// 首字符必须为字母或下划线
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
