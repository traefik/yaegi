/* generic node utility functions */
#include <stdlib.h>
#include <string.h>

#include "node.h"
#include "trace.h"

/* Maintain the following in consistency with enum nodetype_t */
const char *nodetype[NODETYPE_LEN] = {
	"UNDEF", "SL", "IF", "WHILE", "BREAK", "CONTINUE", "DEF", "FOR",
	"RETURN", "LOCAL", "MAP", "OPS", "OP", "TERM", "VAR", "FUN",
	"LVAR", "ARRAY", "COND_BRANCH", "NAMESPACE"
};

/* Recursive implementation of tree walk, depth first */
void tree_walk(bip_t *ip, node_t *node, tw_fun_t in, tw_fun_t out, void *data)
{
	int i;
	if (in)
		in(ip, node, data);
	for (i = 0; node && i < node->nchild; i++)
		tree_walk(ip, node->child[i], in, out, data);
	if (out)
		out(ip, node, data);
}

/* Non recursive tree walk */
void new_tree_walk(bip_t *ip, node_t *node, tw_fun_t in, tw_fun_t out, void *data)
{
	ip->cur = node;
	if (in)
		in(ip, ip->cur, NULL);
	while (1) {
		++ip->cur->visits;
		if (ip->cur->nchild > 0 && ip->cur->visits <= ip->cur->nchild) {
			ip->cur = ip->cur->child[ip->cur->visits - 1];
			if (in)
				in(ip, ip->cur, NULL);
		} else {
			ip->cur->visits = 0;
			if (out)
				out(ip, ip->cur, NULL);
			if (ip->cur == node)
				break;
			else
				ip->cur = ip->cur->anc;
		}
	}
}

/* append child node to the list of anc children node */
void appchild(node_t *anc, node_t *child)
{
	if (child == NULL) {
		//t_s("nil child");
		return;
	}
	anc->child = (node_t **)realloc(anc->child,
					++anc->nchild * sizeof(node_t *));
	anc->child[anc->nchild - 1] = child;
	child->anc = anc;
}

/* insert child node at first position in anc children list */
void inschild(node_t *anc, node_t *child)
{
	node_t **ochild = anc->child;
	int onchild = anc->nchild, i;

	anc->nchild = 0;
	anc->child = NULL;
	appchild(anc, child);
	for (i = 0; i < onchild; i++)
		appchild(anc, ochild[i]);
	free(ochild);
}

void delchild(node_t *anc, node_t *child)
{
	node_t **ochild = anc->child;
	int onchild = anc->nchild, i;

	anc->nchild = 0;
	anc->child = NULL;
	for (i = 0; i < onchild; i++)
		if (ochild[i] != child)
			appchild(anc, ochild[i]);
	free(ochild);
}

void print_val(FILE *fd, val_t *pv, int quote)
{
	switch ((int)pv->type) {
	case VINT:
		fprintf(fd, "%ld", pv->u.num);
		break;
	case VFLOAT:
		fprintf(fd, "%g", pv->u.fnum);
		break;
	case VSTR:
		if (quote == 0)
			fprintf(fd, "\\\"%s\\\"", pv->u.str);
		else if (quote == 2)
			fprintf(fd, "\"%s\"", pv->u.str);
		else
			fprintf(fd, "%s", pv->u.str);
		break;
	default:
		break;
	}
}

void print_node_label(FILE *fd, node_t *node, int flow)
{
	switch ((int)node->type) {
	case TERM:
		fprintf(fd, " %d: ", node->num);
		//fprintf(fd, " ");
		print_val(fd, node->pv, flow);
		break;
	case SL:
		fprintf(fd, " %d: %s", node->num, nodetype[node->type]);
		//fprintf(fd, " %s", nodetype[node->type]);
		break;
	case VAR:
		fprintf(fd, " %d: %s", node->num, node->sym->name);
		//fprintf(fd, " %s", node->sym->name);
		break;
	default:
		if (flow)
			fprintf(fd, " $%d:", node->num);
			//fprintf(fd, " ");
		else
			if (node->sym)
				fprintf(fd, " %d: %s", node->num, node->sym->name);
				//fprintf(fd, " %s", node->sym->name);
			else
				fprintf(fd, " %d: undefined", node->num);
		break;
	}
}

static void pnode(bip_t *ip, node_t *node, void *data)
{
	FILE *fd = (FILE *)data;
	fprintf(fd, "%d [type=\"%s\", label=\"", node->num,
		nodetype[node->type]);
	print_node_label(fd, node, 0);
	fprintf(fd, "\"]\n");
	if (node->anc)
		fprintf(fd, "%d -> %d\n", node->anc->num, node->num);
}

void print_tree(bip_t *ip, FILE *fd, node_t *node)
{
	fprintf(fd, "digraph ast {\n");
	tree_walk(ip, node, pnode, NULL, fd);
	fprintf(fd, "}\n");
}

static void pflow(bip_t *ip, node_t *node, void *data)
{
	int i, start = 0;
	FILE *fd = (FILE *)data;

	if (node == NULL || node->type == TERM || node->type == VAR ||
            node->type == BREAK || node->type == CONTINUE)
		return;
	fprintf(fd, "%d [label=\"%d:", node->num, node->num);
	if (node->type == OP) {
		print_node_label(fd, node->child[0], 1);
		start = 1;
	}
	print_node_label(fd, node, 0);
	for (i = start; i < node->nchild; i++)
		print_node_label(fd, node->child[i], 1);
	fprintf(fd, "\"]\n");
	if (!node->snext)
		return;

	if (node->snext->next[TRUE]) {
		fprintf(fd, "%d -> %d [color=green]\n", node->num,
			node->snext->next[TRUE]->num);
	}
	if (node->snext->next[FALSE]) {
		fprintf(fd, "%d -> %d [color=red]\n", node->num,
			node->snext->next[FALSE]->num);
	}
	if (!node->snext->next[TRUE] && !node->snext->next[FALSE]) {
		fprintf(fd, "%d -> %d\n", node->num, node->snext->num);
	}
}

void print_flow(bip_t *ip, FILE *fd, node_t *node)
{
	int i;

	fprintf(fd, "digraph cfg {\n");
	tree_walk(ip, node, NULL, pflow, fd);
	for (i = 0; i < ip->nentry_points; i++)
		fprintf(fd, "%d [color=red]", ip->entry_points[i]->num);
	fprintf(fd, "}\n");
}

void dotty(bip_t *ip, node_t *node, dotty_fun_t dotty_fun)
{
	FILE *fd = popen("dotty -", "w");
	if (fd == NULL) {
		perror("dotty");
		return;
	}
	dotty_fun(ip, fd, node);
	fflush(fd);
	//getchar();
}

/* Add a new entry (start of thread) for parallel execution */
void addentry(bip_t *ip, node_t *node)
{
	ip->entry = (node_t **)realloc(ip->entry, ++ip->nentry * sizeof(node));
	ip->entry[ip->nentry - 1] = node;
}

/* Add node at end of list, increase list_len */
void appnode(node_t ***list, node_t *node, int *list_len)
{
	*list = (node_t **)realloc(*list, ++(*list_len) * sizeof(node));
	(*list)[*list_len - 1] = node;
}

sym_t *newsym(hash_t *h, const char *s, nodetype_t type, int prio, fun_t ifun,
	      fun_t cfun)
{
	sym_t *sym = (sym_t *)calloc(1, sizeof(*sym));
	sym->name = (char *)s;
	sym->type = type;
	sym->prio = prio;
	sym->f = ifun;
	sym->jf = cfun;
	return (sym_t *)hadd(h, hkey(sym->name), sym->name, sym);
}

sym_t *getsym(bip_t *ip, const char *s, int len)
{
	unsigned int key = hnkey(s, len, 0);
	return (sym_t *)hlookup(ip->gsym, key, (void *)s, len);
}
