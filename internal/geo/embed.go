package geo

import _ "embed"

// packed 内嵌的 gzip 压缩地理数据库原始字节，构建期由 go:embed 打进二进制。
// 数据由 cmd/geo-gen 生成（自定义二进制格式，魔数 "GWG1"），
// 运行时经 load 解压后交给 parse 解析。
//
//go:embed data/geo.bin.gz
var packed []byte
