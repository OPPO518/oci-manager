// Package web 内嵌前端构建产物。
//
// dist 目录由 vite 构建输出到此处（见 web/vite.config.ts 的 outDir），
// 因此最终二进制自带前端，部署时只需分发一个文件。
package web

import "embed"

// DistFS 是前端静态资源。
//
//go:embed all:dist
var DistFS embed.FS
