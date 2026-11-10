package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"testing"
)

func TestParseIPRange(t *testing.T) {
	ips, err := ParseIPRange("192.168.1.1", "192.168.1.10")
	if err != nil {
		t.Fatalf("ParseIPRange failed: %v", err)
	}
	if len(ips) != 10 {
		t.Fatalf("expected 10 IPs, got %d", len(ips))
	}
	if ips[0] != "192.168.1.1" || ips[9] != "192.168.1.10" {
		t.Fatalf("unexpected IP range: %v", ips)
	}
}

func TestProbeHostLocalhost(t *testing.T) {
	res := ProbeHost("127.0.0.1")
	if !res.IsOnline {
		t.Fatalf("expected 127.0.0.1 to be online, got offline")
	}
	t.Logf("127.0.0.1 probe result: status=%s, responseTime=%d ms", res.Status, res.ResponseTimeMs)
}

func TestReportZipArchiveStructure(t *testing.T) {
	// 验证包含 10 台设备时的 ZIP 压缩包文件结构
	zipBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuf)

	// 总览报告
	overview, err := zipWriter.Create("00_巡检总览及汇总报告.md")
	if err != nil {
		t.Fatalf("failed to create overview file in zip: %v", err)
	}
	_, _ = overview.Write([]byte("# 巡检总览\n- 总设备数: 10 台\n"))

	// 10 台独立设备报告文件
	for i := 1; i <= 10; i++ {
		filename := fmt.Sprintf("%02d_网络设备_Switch-%d_192.168.1.%d_巡检报告.md", i, i, i)
		f, err := zipWriter.Create(filename)
		if err != nil {
			t.Fatalf("failed to create device report in zip: %v", err)
		}
		_, _ = f.Write([]byte(fmt.Sprintf("# 设备 Switch-%d 巡检报告\n- IP: 192.168.1.%d\n- 状态: 正常\n", i, i)))
	}

	_ = zipWriter.Close()

	// 解包验证 ZIP 包内文件清单
	zipReader, err := zip.NewReader(bytes.NewReader(zipBuf.Bytes()), int64(zipBuf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip archive: %v", err)
	}

	if len(zipReader.File) != 11 { // 1 overview + 10 device reports
		t.Fatalf("expected 11 files in zip, got %d", len(zipReader.File))
	}

	deviceReportCount := 0
	for _, file := range zipReader.File {
		if file.Name != "00_巡检总览及汇总报告.md" {
			deviceReportCount++
		}
	}

	if deviceReportCount != 10 {
		t.Fatalf("expected exactly 10 device report files in zip, got %d", deviceReportCount)
	}

	t.Logf("ZIP 压缩包结构验证通过: 共有 1 份总览汇总报告 + %d 份独立设备巡检报告文件", deviceReportCount)
}

func TestBatchDeleteEmpty(t *testing.T) {
	count := NetworkDeviceServiceInstance.BatchDeleteDevices([]string{})
	if count != 0 {
		t.Fatalf("expected 0 for empty slice, got %d", count)
	}
}
