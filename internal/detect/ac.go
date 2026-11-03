package detect

// acNode Aho-Corasick 节点。
type acNode struct {
	next map[byte]int
	fail int
	out  []int // 命中的关键词下标
}

// AC 是只读的多模式匹配器，构建后并发安全。
//
// 作用：先用关键词做一遍 O(n) 的廉价预筛，只有命中的规则才去跑正则。
// 没有这一步，200 条正则逐条扫一遍，单请求就会吃掉几百微秒。
type AC struct {
	nodes    []acNode
	keywords []string
}

// NewAC 根据关键词集合构建自动机（关键词会被归一化并去重）。
func NewAC(keywords []string) *AC {
	a := &AC{keywords: make([]string, 0, len(keywords))}
	a.nodes = append(a.nodes, acNode{next: map[byte]int{}})

	for _, kw := range keywords {
		k := Normalize(kw)
		if k == "" {
			continue
		}
		a.keywords = append(a.keywords, k)
		idx := len(a.keywords) - 1

		cur := 0
		for i := 0; i < len(k); i++ {
			c := k[i]
			nxt, ok := a.nodes[cur].next[c]
			if !ok {
				a.nodes = append(a.nodes, acNode{next: map[byte]int{}})
				nxt = len(a.nodes) - 1
				a.nodes[cur].next[c] = nxt
			}
			cur = nxt
		}
		a.nodes[cur].out = append(a.nodes[cur].out, idx)
	}

	a.buildFail()
	return a
}

// buildFail BFS 构建失败指针。
func (a *AC) buildFail() {
	queue := make([]int, 0, len(a.nodes))
	for _, child := range a.nodes[0].next {
		a.nodes[child].fail = 0
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for c, child := range a.nodes[cur].next {
			f := a.nodes[cur].fail
			for {
				if nxt, ok := a.nodes[f].next[c]; ok {
					a.nodes[child].fail = nxt
					break
				}
				if f == 0 {
					a.nodes[child].fail = 0
					break
				}
				f = a.nodes[f].fail
			}
			a.nodes[child].out = append(a.nodes[child].out, a.nodes[a.nodes[child].fail].out...)
			queue = append(queue, child)
		}
	}
}

// Match 返回命中的关键词下标集合；输入应该是已归一化的字符串。
func (a *AC) Match(s string) []int {
	if a == nil || len(a.nodes) == 0 || s == "" {
		return nil
	}
	var hits []int
	cur := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		for {
			if nxt, ok := a.nodes[cur].next[c]; ok {
				cur = nxt
				break
			}
			if cur == 0 {
				break
			}
			cur = a.nodes[cur].fail
		}
		if len(a.nodes[cur].out) > 0 {
			hits = append(hits, a.nodes[cur].out...)
		}
	}
	return hits
}

// Contains 是否存在任一关键词（用于快速判断）。
func (a *AC) Contains(s string) bool {
	return len(a.Match(s)) > 0
}

// Keywords 返回归一化后的关键词表。
func (a *AC) Keywords() []string { return a.keywords }

// Size 返回关键词数量。
func (a *AC) Size() int { return len(a.keywords) }
