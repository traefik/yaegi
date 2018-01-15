#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "node.h"
#include "scan.h"
#include "run_cfg.h"
#include "trace.h"

static node_t *parse_1(bip_t *ip, char **pstr, int *plen);
static node_t *parse_statement(bip_t *ip, char **pstr, int *plen);
node_t *parse_sl(bip_t *ip, char **pstr, int *plen);

static char *strdupn(char *str, int len)
{
	char *s = (char *)malloc(len + 1);
	strncpy(s, str, len);
	s[len] = '\0';
	return s;
}

static node_t *parse_int(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->type = TERM;
	node->num = ++ip->nc;
	node->pv = &node->v;
	node->pv->type = VINT;
	node->pv->u.num = ps->num;
	return node;
}

static node_t *parse_float(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->type = TERM;
	node->num = ++ip->nc;
	node->pv = &node->v;
	node->pv->type = VFLOAT;
	node->pv->u.fnum = ps->fnum;
	return node;
}

static node_t *parse_str(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->type = TERM;
	node->num = ++ip->nc;
	node->pv = &node->v;
	node->pv->type = VSTR;
	node->pv->len = ps->len;
	node->pv->u.str = strdupn(ps->tok, ps->len);
	return node;
}
 
static node_t *parse_parenthesis(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->num = ++ip->nc;
	node->type = SL;
	node->pv = &node->v;
	appchild(node, parse_statement(ip, &ps->tok, &ps->len));
	return node;
}

static node_t *parse_paren(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = parse_statement(ip, &ps->tok, &ps->len);
	node->prio = 20;	// override precedence
	return node;
}

static node_t *parse_bracket(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->num = ++ip->nc;
	node->type = ARRAY;
	appchild(node, parse_statement(ip, &ps->tok, &ps->len));
	return node;
}

static node_t *parse_oper(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->num = ++ip->nc;
	node->pv = &node->v;
	if ((node->sym = getsym(ip, ps->tok, ps->len))) {
		node->type = node->sym->type;
		node->f = node->sym->f;
	}
	if (node->type != OPS)
		appchild(node, parse_1(ip, pstr, plen));
	return node;
}

static node_t *parse_id(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t)), *n;
	scan_t sc;
	char *s;
	int l;

	node->num = ++ip->nc;
	if ((node->sym = getsym(ip, ps->tok, ps->len))) {
		node->type = node->sym->type;
	} else {
		node->type = VAR;
		node->sym = newsym(ip->gsym, strdupn(ps->tok, ps->len),
				   VAR, 0, nop, NULL);
	}
	node->f = node->sym->f;
	switch ((int)node->sym->type) {
	case NAMESPACE:
		appchild(node, parse_1(ip, pstr, plen));
		break;
	case DEF:
		// FIXME: handle all variants of DEF
		// function name
		appchild(node, parse_1(ip, pstr, plen));

		
		// function args
		l = *plen;
		s = *pstr;
		scan(ip->scanfun, &sc, &s, &l);
		appchild(node, parse_parenthesis(ip, &sc, pstr, plen));
		*pstr = s;
		*plen = l;

		// function body
		appchild(node, parse_1(ip, pstr, plen));
		break;
	case FOR:
		// FIXME: handle all variants of FOR
		appchild(node, parse_statement(ip, pstr, plen));
		appchild(node, parse_statement(ip, pstr, plen));
		appchild(node, parse_statement(ip, pstr, plen));
		appchild(node, parse_statement(ip, pstr, plen));
		break;
	case IF:
		appchild(node, parse_statement(ip, pstr, plen));
		appchild(node, parse_statement(ip, pstr, plen));
		s = *pstr;
		l = *plen;
		scan(ip->scanfun, &sc, &s, &l);
		if (sc.type == ID && sc.len == 4 &&
		    strncmp(sc.tok, "else", 4) == 0) {
			*pstr = s;
			*plen = l;
			appchild(node, parse_statement(ip, pstr, plen));
		}
		break;
	case OP:
		appchild(node, parse_1(ip, pstr, plen));
		break;
	case RETURN:
		appchild(node, parse_statement(ip, pstr, plen));
		break;
	case FUN:
		node->pv = &node->v;
		while ((n = parse_1(ip, pstr, plen)))
			appchild(node, n);
		break;
	case VAR:
		node->pv = &node->sym->v;
		s = *pstr;
		l = *plen;
		scan(ip->scanfun, &sc, &s, &l);
		if (sc.type == OPER) {
			sym_t *sym = getsym(ip, sc.tok, sc.len);
			if (sym && sym->type == OPS) {
				n = parse_1(ip, pstr, plen);
				appchild(n, node);
				node = n;
			}
		}
		break;
	default:
		t_s(nodetype[node->sym->type]);
		break;
	}
	return node;
}

static node_t *parse_brace(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	return parse_sl(ip, &ps->tok, &ps->len);
}

static node_t *parse_bad(bip_t *ip, scan_t *ps, char **pstr, int *plen)
{
	return NULL;
}

typedef node_t *(*parse_fun_t) (bip_t *, scan_t *, char **, int *);

// Table indiced by scan_token_t
static const parse_fun_t parse_fun[] = {
	parse_bad, parse_brace, parse_bracket, parse_bad, parse_bad,
	parse_float, parse_id, parse_int, parse_bad, parse_oper,
	parse_paren, parse_str,
};

/* Return a single node, possibly a complex subtree. */
static node_t *parse_1(bip_t *ip, char **pstr, int *plen)
{
	scan_t sc;
	scan(ip->scanfun, &sc, pstr, plen);
	return parse_fun[sc.type](ip, &sc, pstr, plen);
}

/* Return a statement node. Apply operator precedence rules (reordering nodes).  */
static node_t *parse_statement(bip_t *ip, char **pstr, int *plen)
{
	scan_t sc = {};
	char *s = *pstr;
	int len = *plen;
	node_t *first = NULL, *node = NULL, *n, *n1;

start:
	while (len > 0 && scan(ip->scanfun, &sc, &s, &len) != CSEP) {
		if (first && sc.type == BRACE) {
			unscan(&sc, &s, &len);
			break;
		}
		node = parse_fun[sc.type](ip, &sc, &s, &len);
		if (!first) {
			first = node;
		} else if (node->type == OP) {
			node->prio = node->sym->prio;
			for (n = first; n; n = n->nchild > 1 ? n->child[1] : NULL) {
				if (n->type != OP || node->prio <= n->prio
				    || (n->type == OP && n->nchild == 1)) {
					if (n == first) {
						first = node;
					} else {
						n1 = n->anc;
						delchild(n1, n);
						appchild(n1, node);
					}
					inschild(node, n);
					break;
				}
			}
		} else {
			unscan(&sc, &s, &len);
			break;
		}
		// If node is a kind of statement not part of an expression, stop here
		if (node->type < OP)
			break;
	}
	if (!first) {
		// FIXME: handle parse error
		if (len > 0 && sc.type == CSEP) {
			(*pstr)++;
			(*plen)--;
			goto start;
		}
		//t_s(scan_token[sc.type]);
	}
	*pstr = s;
	*plen = len;
	return first;
}

/* Return a statement list node */
node_t *parse_sl(bip_t *ip, char **pstr, int *plen)
{
	node_t *node = (node_t *)calloc(1, sizeof(node_t));
	node->type = SL;
	node->num = ++ip->nc;
	node->sym = getsym(ip, "SL", 2);
	node->f = node->sym->f;
	while (*plen > 0)
		appchild(node, parse_statement(ip, pstr, plen));
	return node;
}
