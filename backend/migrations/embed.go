package migrations

import "embed"

// FS 内嵌版本化的 SQL 迁移文件，使其随二进制一起发布。
//
//go:embed *.sql
var FS embed.FS
