// Package gitignore 解析工作区 .gitignore，提供 git 风格的忽略判定。
// 用于工作区内容哈希与 DSH_HOME 种子复制。匹配器复用
// github.com/git-pkgs/gitignore。
package gitignore

import (
	gogitignore "github.com/git-pkgs/gitignore"
)

// Matcher 是解析后的 gitignore 匹配器（薄封装）。
type Matcher struct {
	m *gogitignore.Matcher
}

// Load 读取 dir 下的 .gitignore（含子目录层级）构造匹配器。没有
// .gitignore 时返回空匹配器。
func Load(dir string) (*Matcher, error) {
	return &Matcher{m: gogitignore.NewFromDirectory(dir)}, nil
}

// Ignored 报告相对路径（/ 分隔）是否被忽略。isDir 用于目录限定 pattern。
func (m *Matcher) Ignored(rel string, isDir bool) bool {
	return m.m.MatchPath(rel, isDir)
}
