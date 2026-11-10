package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func safeExec(db *sql.DB, query string, label string) {
	res, err := db.Exec(query)
	if err != nil {
		fmt.Printf("跳过/不可用 [%s]: %v\n", label, err)
		return
	}
	if res != nil {
		n, _ := res.RowsAffected()
		fmt.Printf("已删除 [%s]: %d 条\n", label, n)
	}
}

func main() {
	db, err := sql.Open("sqlite", "../backend/data/app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== 开始清理假数据 ===")
	safeExec(db, "DELETE FROM network_devices WHERE id LIKE 'dev-%'", "假设备")
	safeExec(db, "DELETE FROM network_inspection_history WHERE device_id LIKE 'dev-%'", "假巡检")
	safeExec(db, "DELETE FROM alerts WHERE device_id LIKE 'dev-%'", "假告警")
	safeExec(db, "DELETE FROM servers WHERE id LIKE 'svr-%'", "假服务器")
	safeExec(db, "DELETE FROM network_subnets WHERE description LIKE '%示例%'", "假子网")
	safeExec(db, "DELETE FROM snmp_interface_metrics WHERE device_id LIKE 'dev-%'", "假指标")

	// 2. 检查真实设备
	rows, err := db.Query("SELECT id, name, ip_address, vendor, model, status, snmp_enabled FROM network_devices")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("\n=== 数据库中保留的真实设备 ===")
	for rows.Next() {
		var id, name, ip string
		var vendor, model, status sql.NullString
		var snmpEnabled sql.NullInt64
		rows.Scan(&id, &name, &ip, &vendor, &model, &status, &snmpEnabled)
		fmt.Printf("ID: %s | 名称: %s | IP: %s | 厂商: %s | 型号: %s | 状态: %s | SNMP: %d\n",
			id, name, ip, vendor.String, model.String, status.String, snmpEnabled.Int64)
	}

	// 3. 检查真实设备的接口指标数量
	var totalMetrics int
	db.QueryRow("SELECT COUNT(*) FROM snmp_interface_metrics").Scan(&totalMetrics)
	fmt.Printf("\n真实设备 SNMP 接口指标总数: %d 条\n", totalMetrics)
}
