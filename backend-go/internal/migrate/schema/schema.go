// Package schema 内嵌数据库 schema，用于全新安装时一次性建成全部表结构。
package schema

import _ "embed"

//go:embed schema_dump.sql
var ConsolidatedSQL string

// MetricsSQL 指标数据库（metrics.db）schema
//go:embed metrics_schema.sql
var MetricsSQL string
