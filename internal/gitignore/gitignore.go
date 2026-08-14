// Package gitignore 解析工作区 .gitignore，提供 git 风格的忽略判定。
//
// 用于两类场景：工作区内容哈希（增量构建判断）与 DSH_HOME 种子复制——
// 哪些内容不属于"项目本身"（构建产物、缓存、本地配置）由工作区的
// .gitignore 表达，工具遵循它而不是硬编码目录名。
//
// 匹配器直接复用 github.com/git-pkgs/gitignore（gitignore 语法边界多，
// 不自造轮子）：NewFromDirectory 会遍历目录读取各层 .gitignore 并按其
// 所在层级限定作用域，与 git 的语义一致。
package gitignore

import (
	gogitignore "github.com/git-pkgs/gitignore"
)

// Matcher 是解析后的 gitignore 匹配器（薄封装）。
type Matcher struct {
	m *gogitignore.Matcher
}

// Load 读取 dir 下的 .gitignore（含子目录层级）构造匹配器。目录不存在
// 或没有 .gitignore 时返回空匹配器（不忽略任何内容）。
func Load(dir string) (*Matcher, error) {
	return &Matcher{m: gogitignore.NewFromDirectory(dir)}, nil
}

// Ignored 报告相对路径（/ 分隔）是否被忽略。isDir 用于目录限定 pattern
// （如尾斜杠模式）。遵循 git 的 last-match-wins，! 取反生效。
func (m *Matcher) Ignored(rel string, isDir bool) bool {
	return m.m.MatchPath(rel, isDir)
}
