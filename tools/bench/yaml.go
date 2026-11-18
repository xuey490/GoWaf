package main

import "gopkg.in/yaml.v3"

// yamlUnmarshal yaml 反序列化的薄封装（隔离 import，便于单处管理）。
func yamlUnmarshal(data []byte, out any) error {
	return yaml.Unmarshal(data, out)
}
