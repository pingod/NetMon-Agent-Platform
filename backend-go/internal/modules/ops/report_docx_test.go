package ops

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestBuildDocx(t *testing.T) {
	md := "# 巡检报告\n\n## 概览\n\n设备 **核心交换机** 状态 `正常`。\n\n" +
		"| 设备 | 状态 |\n| --- | --- |\n| SW-01 | 正常 |\n| SW-02 | 异常 |\n\n" +
		"- 第一项\n- 第二项\n\n1. 步骤一\n2. 步骤二\n\n```\ndevice# show version\n```\n\n---\n\n> 备注：无异常\n"
	data, err := buildDocx("测试报告", md)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	want := map[string]bool{
		"[Content_Types].xml": false, "_rels/.rels": false,
		"word/_rels/document.xml.rels": false, "word/styles.xml": false, "word/document.xml": false,
	}
	var document string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		// 每个部件都必须是良构 XML
		var v interface{}
		if err := xml.Unmarshal(b, &v); err != nil {
			t.Errorf("%s 不是良构 XML: %v", f.Name, err)
		}
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
		if f.Name == "word/document.xml" {
			document = string(b)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("缺少部件 %s", name)
		}
	}
	for _, s := range []string{"测试报告", "巡检报告", "核心交换机", "正常", "SW-01", "第一项", "步骤一", "show version", "备注：无异常"} {
		if !strings.Contains(document, s) {
			t.Errorf("document.xml 缺少内容 %q", s)
		}
	}
	if !strings.Contains(document, "<w:tbl>") {
		t.Error("缺少表格")
	}
	if !strings.Contains(document, `w:val="Heading2"`) {
		t.Error("缺少二级标题样式")
	}
	if strings.Contains(document, "**") || strings.Contains(document, "| --- |") {
		t.Error("Markdown 标记未被转换")
	}
}

func TestParseInline(t *testing.T) {
	runs := parseInline("设备 **SW-01** 状态 `up` 正常")
	if len(runs) != 5 {
		t.Fatalf("runs = %d, want 5: %+v", len(runs), runs)
	}
	if !runs[1].bold || runs[1].text != "SW-01" {
		t.Errorf("bold run 错误: %+v", runs[1])
	}
	if !runs[3].code || runs[3].text != "up" {
		t.Errorf("code run 错误: %+v", runs[3])
	}
}

func TestBuildDocxEscaping(t *testing.T) {
	data, err := buildDocx("A&B <测试>", "含 **<xml> & 符号** 的内容")
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		var v interface{}
		if err := xml.Unmarshal(b, &v); err != nil {
			t.Errorf("%s 转义后仍非法: %v", f.Name, err)
		}
	}
}