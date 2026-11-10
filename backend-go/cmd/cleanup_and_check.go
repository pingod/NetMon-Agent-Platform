package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"itops-backend/internal/paths"
)

// cleanupTarget 一条清理规则：数据库（app/metrics）与删除条件
type cleanupTarget struct {
	db    string
	label string
	query string
}

var cleanupTargets = []cleanupTarget{
	{"app", "假设备", "DELETE FROM network_devices WHERE id LIKE 'dev-%'"},
	{"app", "假巡检", "DELETE FROM network_inspection_history WHERE device_id LIKE 'dev-%'"},
	{"app", "假告警", "DELETE FROM alerts WHERE device_id LIKE 'dev-%'"},
	{"app", "假服务器", "DELETE FROM servers WHERE id LIKE 'svr-%'"},
	{"app", "假子网", "DELETE FROM network_subnets WHERE description LIKE '%示例%'"},
	{"metrics", "假指标", "DELETE FROM snmp_interface_metrics WHERE device_id LIKE 'dev-%'"},
}

func openDB(path string, label string) *sql.DB {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatalf("打开 %s 失败: %v", label, err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("连接 %s 失败: %v", label, err)
	}
	return db
}

func main() {
	confirm := flag.Bool("confirm", false, "执行真实删除（默认仅 dry-run 打印计划）")
	dbPath := flag.String("db", "", "app.db 路径（默认取 DATABASE_PATH / paths.DatabasePath）")
	flag.Parse()

	appPath := *dbPath
	if appPath == "" {
		appPath = paths.DatabasePath()
	}
	metricsPath := filepath.Join(filepath.Dir(appPath), "metrics.db")

	appDB := openDB(appPath, "app.db")
	defer appDB.Close()

	var metricsDB *sql.DB
	if _, err := os.Stat(metricsPath); err == nil {
		metricsDB = openDB(metricsPath, "metrics.db")
		defer metricsDB.Close()
	}

	conns := map[string]*sql.DB{"app": appDB, "metrics": metricsDB}

	fmt.Printf("数据库: %s\n", appPath)
	if metricsDB != nil {
		fmt.Printf("指标库: %s\n", metricsPath)
	}

	if !*confirm {
		fmt.Println("=== DRY-RUN（未修改任何数据，使用 -confirm 执行） ===")
		for _, t := range cleanupTargets {
			db := conns[t.db]
			if db == nil {
				continue
			}
			var n int
			countSQL := strings.Replace(t.query, "DELETE FROM", "SELECT COUNT(*) FROM", 1)
			if i := strings.Index(strings.ToUpper(countSQL), " WHERE "); i > 0 {
				countSQL = countSQL[:i] + countSQL[i:]
			}
			if err := db.QueryRow(countSQL).Scan(&n); err != nil {
				fmt.Printf("无法统计 [%s]: %v\n", t.label, err)
				continue
			}
			fmt.Printf("将删除 [%s]: %d 条\n", t.label, n)
		}
		return
	}

	// 执行前使用 VACUUM INTO 生成一致性备份
	stamp := time.Now().Format("20060102-150405")
	backup := fmt.Sprintf("%s.bak-%s", appPath, stamp)
	if _, err := appDB.Exec(`VACUUM INTO ?`, backup); err != nil {
		log.Fatalf("备份失败，终止清理: %v", err)
	}
	fmt.Printf("已备份到: %s\n", backup)

	fmt.Println("=== 开始清理假数据 ===")
	for _, t := range cleanupTargets {
		db := conns[t.db]
		if db == nil {
			fmt.Printf("跳过 [%s]: 数据库未启用\n", t.label)
			continue
		}
		if err := execInTx(db, t.query); err != nil {
			log.Fatalf("删除 [%s] 失败（已回滚该事务）: %v", t.label, err)
		}
	}

	// 检查真实设备（正确处理 NULL 与扫描错误）
	rows, err := appDB.Query("SELECT id, name, ip_address, vendor, model, status, snmp_enabled FROM network_devices")
	if err != nil {
		log.Fatalf("查询设备失败: %v", err)
	}
	defer rows.Close()

	fmt.Println("\n=== 数据库中保留的真实设备 ===")
	for rows.Next() {
		var id, name, ip string
		var vendor, model, status sql.NullString
		var snmpEnabled sql.NullInt64
		if err := rows.Scan(&id, &name, &ip, &vendor, &model, &status, &snmpEnabled); err != nil {
			log.Fatalf("读取设备行失败: %v", err)
		}
		fmt.Printf("ID: %s | 名称: %s | IP: %s | 厂商: %s | 型号: %s | 状态: %s | SNMP: %d\n",
			id, name, ip, vendor.String, model.String, status.String, snmpEnabled.Int64)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("遍历设备失败: %v", err)
	}

	var totalMetrics int
	if metricsDB != nil {
		if err := metricsDB.QueryRow("SELECT COUNT(*) FROM snmp_interface_metrics").Scan(&totalMetrics); err != nil {
			log.Printf("统计接口指标失败: %v", err)
		}
	}
	fmt.Printf("\n真实设备 SNMP 接口指标总数: %d 条\n", totalMetrics)
}

// execInTx 在事务内执行删除并返回受影响行数
func execInTx(db *sql.DB, query string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	res, err := tx.Exec(query)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("已删除: %d 条\n", n)
	return nil
}
