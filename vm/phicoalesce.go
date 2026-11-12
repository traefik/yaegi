package vm

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Phi coalescing: a value that flows into a phi gets the register of the
// phi.
//
// The phis of go/ssa become moves on the incoming edges (edge): the loop
// `for i := 0; i < n; i++ { s += x }` ran two OpMov per iteration, for s
// and for i. When the incoming value is computed in the predecessor
// block for the phi alone, and nothing in that block reads the phi after
// it, the value can be written into the register of the phi and the move
// disappears (the simple cases of out-of-SSA coalescing, Boissinot et
// al., CGO 2009). VM_OPT=-phi keeps the moves.
//
// The conditions, for a phi p of block B and its value v on the edge
// from the block P:
//   - v is an instruction of P, not a phi, not a tuple nor an Extract,
//     not a complex number (several instructions), and p is its only
//     referrer;
//   - B is the only successor of P: on another path out of P, p would
//     still be read with its old value;
//   - no instruction of P after v reads p: it would read v;
//   - p is not the value of another phi of B on that edge: the parallel
//     copy would read the new value.
//
// Two SSA values then share a register. gclive.go keeps a register while
// any of its values is live (clearDead), and no instruction of the VM
// writes its destination before it reads an operand of the same kind.

// coalescePhis gives the registers of the phis to the values that can
// take them. It runs before the blocks are compiled.
func (c *compiler) coalescePhis() {
	if !optPhi {
		return
	}
	for _, b := range c.f.ssa.Blocks {
		var phis []*ssa.Phi
		for _, ins := range b.Instrs {
			phi, ok := ins.(*ssa.Phi)
			if !ok {
				break
			}
			phis = append(phis, phi)
		}
		if len(phis) == 0 {
			continue
		}
		for i, p := range b.Preds {
			if len(p.Succs) != 1 {
				continue
			}
			// The values that the phis of b read on this edge.
			read := map[ssa.Value]bool{}
			for _, phi := range phis {
				read[phi.Edges[i]] = true
			}
			for _, phi := range phis {
				v := phi.Edges[i]
				if read[phi] || !c.coalescible(v, phi, p) {
					continue
				}
				if _, ok := c.regs[v]; ok {
					continue // a parameter, a free variable, or taken already
				}
				c.regs[v] = c.reg(phi)
			}
		}
	}
}

// coalescible reports whether the value v, read by phi on the edge from
// pred, can be written into the register of phi.
func (c *compiler) coalescible(v ssa.Value, phi *ssa.Phi, pred *ssa.BasicBlock) bool {
	ins, ok := v.(ssa.Instruction)
	if !ok || ins.Block() != pred || c.skip[ins] {
		return false
	}
	switch v.(type) {
	case *ssa.Phi, *ssa.Extract:
		return false
	}
	if _, ok := v.Type().(*types.Tuple); ok || isComplex(v.Type()) {
		return false
	}
	refs := v.Referrers()
	if refs == nil || len(*refs) != 1 || (*refs)[0] != phi {
		return false
	}
	after := false
	for _, x := range pred.Instrs {
		if x == ins {
			after = true
			continue
		}
		if !after {
			continue
		}
		for _, op := range x.Operands(nil) {
			if op != nil && *op == phi {
				return false
			}
		}
	}
	return true
}
