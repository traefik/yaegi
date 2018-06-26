/* jit runtime for bip */

#include <stdio.h>
#include <lightning.h>

#include "node.h"
#include "run_jit.h"
#include "trace.h"

typedef struct reg_t {
	sym_t *sym;	/* symbol associated to register */
} reg_t;

static reg_t regs[JIT_V_NUM];

static jit_state_t *_jit;	/* XXX: should be in bip_t */

static inline void load(int reg, node_t *n)
{
	if (n->type == VAR)
		n->reg = n->sym->reg;
	if (n->reg) {
		if (reg != n->reg)
			jit_movr(reg, n->reg);
	} else if (n->type == TERM)
		jit_movi(reg, n->pv->u.num);
	else if (reg != JIT_R0)
		jit_movr(reg, JIT_R0);
	//else if (0)
	//	jit_ldi(reg, &n->pv->u.num);
}

static inline int is_if_cond(node_t *n)
{
	return (n->anc && n->anc->type == IF && n->anc->child[0] == n);
}

static inline int is_for_cond(node_t *n)
{
	return (n->anc && n->anc->type == FOR && n->anc->child[1] == n);
}

static inline int is_while_cond(node_t *n)
{
	return (n->anc && n->anc->type == WHILE && n->anc->child[0] == n);
}

static inline int is_cond(node_t *n)
{
	return is_if_cond(n) || is_for_cond(n) || is_while_cond(n);
}

static int regalloc(sym_t *sym)
{
	int i;
	if (sym->reg)
		return sym->reg;		/* already allocated */
	for (i = 0; i < JIT_V_NUM; i++) {
		if (regs[i].sym == NULL) {
			regs[i].sym = sym;
			sym->reg = JIT_V(i);
			jit_ldi(sym->reg, &sym->v.u.num);
			return i;
		}
	}
	return 0;
}

void j_assign(node_t *n)
{
	sym_t *var = n->child[0]->sym;

	if (regalloc(var)) {
		n->reg = n->child[0]->reg = var->reg;
		load(n->reg, n->child[1]);
	} else {
		load(JIT_R0, n->child[1]);
		jit_sti(&n->child[0]->pv->u.num, JIT_R0);
	}
	return;
}

static int gen_branch(node_t *n, int r1)
{
	jit_node_t **plabel;
	node_t *c1;

	if (!is_cond(n))
		return 0;
	plabel = (n->anc->nchild == 3) ?
			&n->anc->child[2]->label :
			&n->anc->label;
	c1 = n->child[1];
	if (n->sym->jf == j_eq) {
		*plabel = (c1->type == TERM) ?
				jit_bnei(r1, c1->pv->u.num) :
				jit_bner(r1, c1->reg);
	} else if (n->sym->jf == j_ne) {
		*plabel = (c1->type == TERM) ?
				jit_beqi(r1, c1->pv->u.num) :
				jit_beqr(r1, c1->reg);
	} else if (n->sym->jf == j_le) {
		*plabel = (c1->type == TERM) ?
				jit_bgti(r1, c1->pv->u.num) :
				jit_bgtr(r1, c1->reg);
	} else if (n->sym->jf == j_gt) {
		*plabel = (c1->type == TERM) ?
				jit_blei(r1, c1->pv->u.num) :
				jit_bler(r1, c1->reg);
	} else if (n->sym->jf == j_lt) {
		*plabel = (c1->type == TERM) ?
				jit_bgei(r1, c1->pv->u.num) :
				jit_bger(r1, c1->reg);
	} else if (n->sym->jf == j_ge) {
		*plabel = (c1->type == TERM) ?
				jit_blti(r1, c1->pv->u.num) :
				jit_bltr(r1, c1->reg);
	} else {
		t_s(n->sym->name);
		*plabel = jit_beqi(r1, 0);
	}
	return 1;
}

#define define_binary_operator(name)					\
void j_##name(node_t *n)						\
{									\
	int r0, r1;							\
	if (n->anc->type == VAR)					\
		n->reg = n->anc->reg = n->anc->sym->reg;		\
	else if (n->anc->type == OP && n->anc->sym->jf == j_assign)	\
		n->reg = n->anc->reg = n->anc->child[0]->sym->reg;	\
	if (n->child[0]->type == VAR)					\
		n->child[0]->reg = n->child[0]->sym->reg;		\
	if (n->child[1]->type == VAR)					\
		n->child[1]->reg = n->child[1]->sym->reg;		\
	r0 = n->anc->reg;						\
	r1 = n->child[0]->reg;						\
	load(r1, n->child[0]);						\
	if (gen_branch(n, r1))						\
		return;							\
	if (n->child[1]->reg) {						\
		jit_##name##r(r0, r1, JIT_V(n->child[1]->reg));		\
	} else if (n->child[1]->type == TERM) {				\
		jit_##name##i(r0, r1, n->child[1]->pv->u.num);		\
	} else {							\
		jit_##name##r(r0, r1, JIT_R0);				\
	}								\
	if (0) jit_sti(&n->pv->u.num, JIT_V(n->anc->reg));		\
}

define_binary_operator(add)	/* c0 + c1 */
define_binary_operator(sub)	/* c0 - c1 */
define_binary_operator(mul)	/* c0 * c1 */
define_binary_operator(div)	/* c0 / c1 */
define_binary_operator(rem)	/* c0 % c1 */
define_binary_operator(lt)	/* c0 < c1 */
define_binary_operator(le)	/* c0 <= c1 */
define_binary_operator(eq)	/* c0 == c1 */
define_binary_operator(ne)	/* c0 != c1 */
define_binary_operator(gt)	/* c0 > c1 */
define_binary_operator(ge)	/* c0 >= c1 */
define_binary_operator(and)	/* c0 & c1 */
define_binary_operator(or)	/* c0 | c1 */
define_binary_operator(xor)	/* c0 ^ c1 */
define_binary_operator(lsh)	/* c0 << c1 */
define_binary_operator(rsh)	/* c0 >> c1 */

#define define_unary_operator(name)			\
void j_##name(node_t *n)				\
{							\
	jit_ldi(JIT_V0, &n->child[0]->pv->u.num);	\
	jit_##name##r(JIT_V0, JIT_V0);			\
	jit_sti(&n->pv->u.num, JIT_V0);			\
}

define_unary_operator(neg)
define_unary_operator(com)

static void do_echo_i(int i)
{
	printf("%d\n", i);
}

void j_echo(node_t *n)
{
	jit_prepare();
	load(JIT_R0, n->child[0]);
	jit_pushargr(JIT_R0);
	jit_finishi((jit_pointer_t)do_echo_i);
}

void j_land(node_t *n)
{
	load(JIT_R0, n->child[0]);
	n->label = jit_beqi(JIT_R0, 0);
	load(JIT_R0, n->child[1]);
	jit_patch(n->label);
}

void j_lor(node_t *n)
{
	load(JIT_R0, n->child[0]);
	n->label = jit_bnei(JIT_R0, 0);
	load(JIT_R0, n->child[1]);
	jit_patch(n->label);
}

void j_nop(node_t *n)
{
	return;
}

void j_inc(node_t *n)
{
	// Avoid costly (and useless in loops) load/store between memory and register
	//jit_ldi(JIT_V0, &n->child[0]->pv->u.num);
	//jit_addi(JIT_V0, JIT_V0, 1);
	//jit_sti(&n->child[0]->pv->u.num, JIT_V0);
	int reg = n->child[0]->sym->reg;
	jit_addi(reg, reg, 1);
}

void j_not(node_t *n)
{
	load(JIT_R0, n->child[0]);
	jit_eqi(JIT_R0, JIT_R0, 0);
	jit_sti(&n->pv->u.num, JIT_R0);
}

/* code generated by postorder traveral of the abstract syntax tree */
static void code_generation(bip_t *ip, node_t *n, void *data)
{
	node_t *anc = n->anc;

	if (is_leaf(n)) {
		if (is_cond(n)) {
			load(JIT_R0, n);
			if (anc->nchild == 3)
				anc->child[2]->label = jit_beqi(JIT_R0, 0);
			else
				anc->label = jit_beqi(JIT_R0, 0);
		}
		return;
	}
	if (n->label)
		jit_patch(n->label);
	if (is_for_cond(n))
		n->label = jit_label();
	if (is_while_cond(n))
		n->label = jit_label();
	n->sym->jf(n);

	if (anc == NULL)
		return;
	if (anc->type == IF) {
		if (anc->child[1] == n) {	/* in true block */
			if (anc->nchild == 3)
				anc->child[1]->label = jit_jmpi();
		} else if (anc->nchild == 3 && anc->child[2] == n) { /* in false block */
			jit_patch_at(n->anc->child[1]->label, jit_label());
		}
	} else if (anc->type == FOR) {
		if (anc->child[3] == n)		/* in true block */
			jit_patch_at(jit_jmpi(), anc->child[1]->label);
	} else if (anc->type == WHILE) {
		if (anc->child[1] == n)		/* in true block */
			jit_patch_at(jit_jmpi(), anc->child[0]->label);
	}
}

/* compute necessary space for stack allocation, tag nodes using stack */
static void stack_analysis(bip_t *ip, node_t *n, void *data)
{
	if (n->type == OP) {
		t_s(n->sym->name);
		if (!is_leaf(n->child[0])) {
			t_d(n->num);
			t_d(ip->prev->num);
			t_d(n->child[0]->num);
			t_s(n->child[0]->sym->name);
		}
		if (!is_leaf(n->child[1])) {
			t_d(n->num);
			t_d(ip->prev->num);
			t_d(n->child[1]->num);
			t_s(n->child[1]->sym->name);
		}
	}
	ip->prev = n;
}

void run_jit(bip_t *ip, node_t *node)
{
	void (*func)(void);

	init_jit("b1");
	_jit = jit_new_state();

	jit_prolog();
	tree_walk(ip, node, NULL, stack_analysis, NULL);
	tree_walk(ip, node, NULL, code_generation, NULL);
	jit_epilog();

	func = (void (*)(void))jit_emit();
	if (ip->opt.v) {
		jit_print();
		jit_disassemble();
	}
	if (!ip->opt.n)
		func();
	finish_jit();
}
