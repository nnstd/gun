package ssa

// ComputeDominators computes the immediate dominator tree for a function's CFG
// using the Cooper-Harvey-Kennedy algorithm (a simplified variant of Lengauer-Tarjan).
//
// After this call, every Block has its IDom and Children fields populated.
func ComputeDominators(fn *Function) {
	if len(fn.Blocks) == 0 {
		return
	}
	entry := fn.Blocks[0]
	entry.IDom = entry // entry dominates itself

	// Build reverse postorder numbering
	rpo := reversePostorder(fn)
	rpoNum := make(map[*Block]int, len(rpo))
	for i, b := range rpo {
		rpoNum[b] = i
	}

	changed := true
	for changed {
		changed = false
		for _, b := range rpo[1:] { // skip entry
			var newIdom *Block
			for _, p := range b.Preds {
				if p.IDom == nil {
					continue // not yet processed
				}
				if newIdom == nil {
					newIdom = p
				} else {
					newIdom = intersect(newIdom, p, rpoNum)
				}
			}
			if newIdom != nil && b.IDom != newIdom {
				b.IDom = newIdom
				changed = true
			}
		}
	}

	// Build children lists
	for _, b := range fn.Blocks {
		b.Children = nil
	}
	for _, b := range fn.Blocks {
		if b.IDom != nil && b.IDom != b {
			b.IDom.Children = append(b.IDom.Children, b)
		}
	}
}

// intersect finds the common dominator of two blocks using the RPO numbering.
func intersect(b1, b2 *Block, rpoNum map[*Block]int) *Block {
	for b1 != b2 {
		for rpoNum[b1] > rpoNum[b2] {
			b1 = b1.IDom
		}
		for rpoNum[b2] > rpoNum[b1] {
			b2 = b2.IDom
		}
	}
	return b1
}

// ComputeDominanceFrontiers computes the dominance frontier for each block.
// Requires that ComputeDominators has already been called.
func ComputeDominanceFrontiers(fn *Function) {
	for _, b := range fn.Blocks {
		b.DomFront = nil
	}

	for _, b := range fn.Blocks {
		if len(b.Preds) < 2 {
			continue
		}
		for _, p := range b.Preds {
			runner := p
			for runner != nil && runner != b.IDom {
				// Add b to runner's dominance frontier
				found := false
				for _, df := range runner.DomFront {
					if df == b {
						found = true
						break
					}
				}
				if !found {
					runner.DomFront = append(runner.DomFront, b)
				}
				runner = runner.IDom
			}
		}
	}
}

// reversePostorder returns blocks in reverse postorder traversal.
func reversePostorder(fn *Function) []*Block {
	if len(fn.Blocks) == 0 {
		return nil
	}
	visited := make(map[*Block]bool)
	var order []*Block

	var dfs func(b *Block)
	dfs = func(b *Block) {
		if visited[b] {
			return
		}
		visited[b] = true
		for _, s := range b.Succs {
			dfs(s)
		}
		order = append(order, b)
	}

	dfs(fn.Blocks[0])

	// Reverse
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}
