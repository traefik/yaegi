/*
 * Generate a serial control flow graph from an abstract syntax tree
 */
#include <stdlib.h>

#include "node.h"
#include "trace.h"

static node_t *add_cond_branch(bip_t *ip, node_t *n)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->type = COND_BRANCH;
	node->num = ++ip->nc;
	node->pv = n->pv;
	node->sym = getsym(ip, "CB", 2);
	node->f = node->sym->f;
	return node;
}

static void cfg_out(bip_t *ip, node_t *node, void *data)
{
	int i;
	node_t *n;

	switch ((int)node->type) {
	case FUN: case OP: case OPS: case SL:
		for (i = 0; i < node->nchild; i++) {
			if (!is_leaf(node->child[i])) {
				node->start =  node->child[i]->start;
				break;
			}
		}
		if (!node->start)
			node->start = node;
		for (i = 1; i < node->nchild; i++) {
			node->child[i-1]->snext = node->child[i]->start;
		}
		for (i = node->nchild - 1; i >= 0; i--) {
			if (!is_leaf(node->child[i])) {
				node->child[i]->snext = node;
				break;
			}
		}
		break;
	case IF:
		node->start = node->child[0]->start;
		node->child[1]->snext = node;
		if (node->nchild == 3) {
			node->child[2]->snext = node;
		}
		n = add_cond_branch(ip, node->child[0]);
		node->child[0]->snext = n;
		n->next[TRUE] =  node->child[1]->start;
		if (node->nchild == 3) {
			n->next[FALSE] = node->child[2]->start;
		} else {
			n->next[FALSE] = node;
		}
		break;
	case WHILE:
		node->start = node->child[0]->start;
		node->child[1]->snext = node->start;
		n = add_cond_branch(ip, node->child[0]);
		node->child[0]->snext = n;
		n->next[TRUE] =  node->child[1]->start;
		n->next[FALSE] = node;
		break;
	case FOR:
		node->start = node->child[0]->start;
		node->child[0]->snext = node->child[1]->start;
		n = add_cond_branch(ip, node->child[1]);
		node->child[1]->snext = n;
		n->next[TRUE] = node->child[3]->start;
		n->next[FALSE] = node;
		node->child[3]->snext = node->child[2]->start;
		node->child[2]->snext = node->child[1]->start;
		break;
	default:
		; //t_s(nodetype[node->type]);
	}
}

void ast2cfg(bip_t *ip, node_t *node)
{
	tree_walk(ip, node, NULL, cfg_out, NULL);
	addentry(ip, node->start);
}
