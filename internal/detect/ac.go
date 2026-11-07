package detect

// acNode Aho-Corasick 自动机的节点。
type acNode struct {
	next map[byte]int // 子节点表：字节 → 子节点下标（按字节匹配，无需处理多字节字符）
	fail int          // 失败指针：失配时回退到的节点下标，指向已匹配串的最长真后缀
	out  []int        // 该节点上命中的关键词下标（构建期已合并失败链上的后缀命中）
}

// AC 是只读的多模式匹配器，构建后并发安全。
//
// 作用：先用关键词做一遍 O(n) 的廉价预筛，只有命中的规则才去跑正则。
// 没有这一步，200 条正则逐条扫一遍，单请求就会吃掉几百微秒。
type AC struct {
	nodes    []acNode // 自动机节点数组，nodes[0] 为根节点
	keywords []string // 归一化后的关键词表，下标与节点 out 中的编号一一对应
}

// NewAC 根据关键词集合构建 Aho-Corasick 自动机。
// 先把每个关键词 Normalize 归一化（空串跳过；不做去重，重复关键词各占一个下标），
// 逐条插入 Trie 后再 BFS 构建失败指针。
// 参数 keywords：原始关键词集合，通常来自各条规则的匹配关键字。
// 返回 *AC：构建完成的只读自动机，可并发使用。
func NewAC(keywords []string) *AC {
	a := &AC{keywords: make([]string, 0, len(keywords))}
	// 根节点固定为 0 号
	a.nodes = append(a.nodes, acNode{next: map[byte]int{}})

	// 逐条把关键词插入 Trie：沿已有边下走，缺失的边就新建节点
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
		// 在关键词终点的节点上登记其下标，扫描走到这里即视为命中
		a.nodes[cur].out = append(a.nodes[cur].out, idx)
	}

	a.buildFail()
	return a
}

// buildFail BFS 层序构建失败指针，并把失败链上的命中合并进各节点的 out。
// BFS 保证处理到某节点时其失败指针指向的浅层节点已构建完成，out 的合并可以一次到位。
// 无参数、无返回值，仅在 NewAC 构建阶段调用一次。
func (a *AC) buildFail() {
	queue := make([]int, 0, len(a.nodes))
	// 根节点的直接子节点（深度为 1）失败指针一律指向根
	for _, child := range a.nodes[0].next {
		a.nodes[child].fail = 0
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for c, child := range a.nodes[cur].next {
			// 沿父节点的失败指针链回退，找到第一个拥有字节 c 这条边的祖先，
			// 该祖先的 c 子节点即为 child 的失败指针；一路退到根都没有则指向根
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
			// 失败节点能命中的关键词本节点也能命中（更短的后缀），一并合并
			a.nodes[child].out = append(a.nodes[child].out, a.nodes[a.nodes[child].fail].out...)
			queue = append(queue, child)
		}
	}
}

// Match 在文本中查找所有出现过的关键词。
// 参数 s：待匹配文本，应为已 Normalize 的字符串（关键词构建时同样归一化，两者口径才一致）。
// 返回 []int：命中的关键词下标集合（按扫描顺序追加、可能重复），无命中时返回 nil。
func (a *AC) Match(s string) []int {
	if a == nil || len(a.nodes) == 0 || s == "" {
		return nil
	}
	var hits []int
	cur := 0 // 当前所在节点，从根开始
	for i := 0; i < len(s); i++ {
		c := s[i]
		// 当前节点没有字节 c 这条边时沿失败指针回退，直到能走或退回根
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
		// out 非空说明有以当前字符结尾的关键词命中（含更短的后缀关键词）
		if len(a.nodes[cur].out) > 0 {
			hits = append(hits, a.nodes[cur].out...)
		}
	}
	return hits
}

// Contains 快速判断文本中是否命中至少一个关键词。
// 参数 s：待匹配文本（应为已归一化的字符串）。
// 返回 bool：存在任一关键词命中时为 true。
func (a *AC) Contains(s string) bool {
	return len(a.Match(s)) > 0
}

// Keywords 返回归一化后的关键词表。
// 返回 []string：构建时登记的关键词（已归一化），下标与 Match 结果对应。
func (a *AC) Keywords() []string { return a.keywords }

// Size 返回关键词数量。
// 返回 int：自动机中登记的关键词个数。
func (a *AC) Size() int { return len(a.keywords) }
