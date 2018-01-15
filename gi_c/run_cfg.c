#include <stdarg.h>
#include <stdlib.h>
#include <string.h>

#include "node.h"
#include "trace.h"

#define FALSE	0
#define TRUE	1

void nop(node_t *n)
{
	return;
}

void assign(node_t *n)
{
	*n->pv = *n->child[0]->pv = *n->child[1]->pv;
}

void cond_branch(node_t *n)
{
	n->snext = n->pv->u.num ? n->next[TRUE] : n->next[FALSE];
}

void inc(node_t *n)
{
	n->child[0]->pv->u.num++;
}

void bprintf_val(val_t *pv)
{
	switch((int)pv->type) {
	case VINT:
		printf("%ld", pv->u.num);
		break;
	case VFLOAT:
		printf("%g", pv->u.fnum);
	case VSTR:
		printf("%s", pv->u.str);
	default:
		break;
	}
}

void bprint_val(bip_t *ip, val_t *pv, int quote)
{
	char s[100];

	switch ((int)pv->type) {
	case VINT:
		snprintf(s, sizeof(s), "%ld", pv->u.num);
		break;
	case VFLOAT:
		snprintf(s, sizeof(s), "%g", pv->u.fnum);
		break;
	case VSTR:
		snprintf(s, sizeof(s), "%s", pv->u.str);
		break;
		if (quote == 0)
			snprintf(s, sizeof(s), "\\\"%s\\\"", pv->u.str);
		else if (quote == 2)
			snprintf(s, sizeof(s), "\"%s\"", pv->u.str);
		else
			snprintf(s, sizeof(s), "%s", pv->u.str);
		break;
	default:
		break;
	}
	strcat(ip->out, s);
}

void bprint_node_label(bip_t *ip, node_t *node, int flow)
{
	char s[100];

	switch ((int)node->type) {
	case TERM:
		strcat(ip->out, " ");
		bprint_val(ip, node->pv, flow);
		break;
	case SL:
		snprintf(s, sizeof(s), " %s", nodetype[node->type]);
		strcat(ip->out, s);
		break;
	case VAR:
		snprintf(s, sizeof(s), " %s", node->sym->name);
		strcat(ip->out, s);
		break;
	default:
		if (flow)
			snprintf(s, sizeof(s), " $%d", node->num);
		else
			snprintf(s, sizeof(s), " %s", node->sym->name);
		strcat(ip->out, s);
		break;
	}
}

void echo(node_t *n)
{
	int i;
	for (i = 0; i < n->nchild; i++)
		bprintf_val(n->child[i]->pv);
	printf("\n");
		//bprint_val(ip, n->child[i]->pv, 1);
	//strcat(ip->out, "\n");
}

void trace(FILE *fd, bip_t *ip, node_t *node, int tid)
{
	int i, start = 0;
	char s[100];

	if (node->sym->f == nop || node->type == SL)
		return;
	snprintf(s, sizeof(s), "[%d] $%d:", tid, node->num);
	strcat(ip->out, s);
	if (node->type == OP) {
		bprint_node_label(ip, node->child[0], 2);
		start = 1;
	}
	bprint_node_label(ip, node, 0);
	for (i = start; i < node->nchild; i++)
		bprint_node_label(ip, node->child[i], 2);
	strcat(ip->out, ": ");
	bprint_val(ip, node->pv, 2);
	strcat(ip->out, "\n");
}

void run_cfg(bip_t *ip)
{
	node_t *n = ip->entry[0];
	while (n) {
		n->f(n);
		n = n->snext;
	}
}

#define define_arithmetic_opfun(name, op)				\
void name(node_t *n)							\
{									\
	val_t *v0, *v1;							\
	const static val_t v = {0};					\
	if (n->nchild == 1) {						\
		v0 = (val_t *)&v;					\
		v1 = n->child[0]->pv;					\
	} else {							\
		v0 = n->child[0]->pv;					\
		v1 = n->child[1]->pv;					\
	}								\
	n->pv->u.num = v0->u.num op v1->u.num;				\
}

#define define_comparison_opfun(name, op)				\
void name(node_t *n)							\
{									\
	n->pv->u.num = n->child[0]->pv->u.num op n->child[1]->pv->u.num;\
}

#define define_bitwise_opfun(name, op)					\
void name(node_t *n)							\
{									\
	n->pv->u.num = n->child[0]->pv->u.num op n->child[1]->pv->u.num;\
}

/* Generate code for common operators */
#define check_zero_div(v)	\
	{if (v == 0) fprintf(stderr, "run error: divide by zero\n"); return;}
define_arithmetic_opfun(fdiv, /)
#undef check_zero_div
#define check_zero_div(v)
define_arithmetic_opfun(add, +)
define_arithmetic_opfun(sub, -)
define_arithmetic_opfun(mul, *)
define_bitwise_opfun(band, &)
define_bitwise_opfun(bor, |)
define_bitwise_opfun(lshift, <<)
define_bitwise_opfun(rshift, >>)
define_bitwise_opfun(mod, %)
define_comparison_opfun(eq, ==)
define_comparison_opfun(neq, !=)
define_comparison_opfun(ge, >=)
define_comparison_opfun(gt, >)
define_comparison_opfun(le, <=)
define_comparison_opfun(lt, <)
