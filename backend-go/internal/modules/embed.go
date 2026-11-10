// 嵌入前端构建产物（dist/）到 Go 二进制
// 部署时只需一个可执行文件，无需额外的静态文件目录
package modules

import (
	"embed"
	"io/fs"
)

//go:embed frontend_dist/*
var frontendEmbed embed.FS

// FrontendFS 返回嵌入的前端文件系统（已剥离 "frontend_dist" 前缀）
func FrontendFS() fs.FS {
	sub, err := fs.Sub(frontendEmbed, "frontend_dist")
	if err != nil {
		return nil
	}
	return sub
}
