#ifndef NODE_H
#define NODE_H

#include <pthread.h>
#include <stdio.h>

#include "hash.h"
#include "scan.h"

#ifndef TRUE
#define TRUE 1
#define FALSE 0
#endif

#define UP 0
#define DOWN 1
#define RD 0
#define WR 1

// types which could be part of expressions must be defined after OP
typedef enum nodetype_t {
	UNDEF, SL, IF, WHILE, BREAK, CONTINUE, DEF, FOR, RETURN, LOCAL, MAP, OPS,
	OP, TERM, VAR, FUN, LVAR, ARRAY, COND_BRANCH, NAMESPACE,
	NODETYPE_LEN
} nodetype_t;

// String version of above, for debug
extern const char *nodetype[NODETYPE_LEN];

typedef enum valtype_t {
	VINT, VSTR, VFUN, VTAB, VPTR, VFLOAT, VSYM, VVAR, VPINT, VPFLOAT,
	VVOID, VSHORT, VQUAD, VBIN
} valtype_t;

typedef struct bip_t bip_t;
typedef struct node_t node_t;
typedef void (*fun_t)(node_t *n);
typedef struct jit_node jit_node_t;	/* from lightning.h */

typedef struct val_t {
	int len;
	valtype_t type;			/* type of following field */
	union {
		long num;		/* integer value */
		double fnum;		/* floating point value */
		char *str;		/* string value */
		void *data;		/* user data value */
		// unsigned char *bin;	/* binary data */
		// struct sym_t *sym;	/* symbol value */
		// fun_t fun;		/* function value */
	} u;
} val_t;

typedef struct sym_t {
	char *name;			/* symbol unique name */
	nodetype_t type;		/* symbol type */
	int prio;			/* operator priority */
	node_t *assign;			/* node which sets this symbol */
	val_t v;			/* data value */
	fun_t f;			/* function to run during execution */
	fun_t jf;			/* jit compiler callback */
	node_t **access[2];
	int naccess[2];
	int reg;			/* jit register number */
} sym_t;

struct node_t {
	node_t *anc;			/* unique ancestor */
	node_t **child;			/* array of children */
	sym_t *sym;			/* symbol (variable, function, op) */
	nodetype_t type;
	int num;			/* unique serial number */
	int nchild;			/* number of children */
	int prio;			/* operator precedence (from sym) */
	val_t v;			/* value during execution */
	val_t *pv;			/* pointer on above or sym value */
	fun_t f;			/* function to run during execution */
	int visits;     /* Number of node visits for non rec tree_walk */

	node_t *start;			/* entry point in subtree (CFG) */
	node_t *snext;			/* next node to eval (CFG) */
	node_t *next[2];		/* false and true branches in CFG */

	jit_node_t *label;		/* label to jump to (JIT) */
	int reg;			/* register number (JIT) */
};

/* Interpreter state, used  during compilation and execution */
struct bip_t {
	scan_fun_t *scanfun;    /* lexical scanner table of functions */
	hash_t *gsym;		/* parse: global hashed symbol table */
	hash_t *lsym;		/* parse: local hashed symbol table */
	node_t *root;
	node_t *cur;
	node_t *prev;
	node_t **entry;		/* run: array of entry points */
	int nentry;		/* number of entry points (threads) */
	int nc;			/* node counter */
	int last_direction;
	node_t *fork_node;       /* To replace jump stack */
	node_t *ctx_node;        /* context ID */
	node_t **entry_points;
	int nentry_points;
	node_t *global_fork_node;
	struct {
		unsigned int a : 1;	/* AST option */
		unsigned int c : 1;	/* CFG option */
		unsigned int n : 1;	/* no-run option */
		unsigned int p : 1;	/* parallel option */
		unsigned int v : 1;	/* trace option */
		unsigned int x : 1;	/* native exec option */
	} opt;
	FILE *fd_ast;		/* file to write AST */
	FILE *fd_cfg;		/* file to write CFG */
	char *out;
	int outlen;
	pthread_mutex_t outm;
};

#define is_leaf(node)		((node)->type == TERM || (node)->type == VAR)

typedef void (*tw_fun_t)(bip_t *, node_t *, void *data);
typedef void (*dotty_fun_t)(bip_t *ip, FILE *fd, node_t *node);

void tree_walk(bip_t *ip, node_t *node, tw_fun_t in, tw_fun_t out, void *data);
void new_tree_walk(bip_t *ip, node_t *node, tw_fun_t in, tw_fun_t out, void *data);
void appchild(node_t *anc, node_t *child);
void inschild(node_t *anc, node_t *child);
void delchild(node_t *anc, node_t *child);
void print_val(FILE *fd, val_t *pv, int quote);
void print_node_label(FILE *fd, node_t *node, int flow);
void print_tree(bip_t *ip, FILE *fd, node_t *node);
void print_flow(bip_t *ip, FILE *fd, node_t *node);
void dotty(bip_t *ip, node_t *node, dotty_fun_t dotty_fun);
void addentry(bip_t *ip, node_t *node);
void appnode(node_t ***list, node_t *node, int *list_len);
sym_t *newsym(hash_t *h, const char *s, nodetype_t type, int prio, fun_t ifun, fun_t cfun);
sym_t *getsym(bip_t *ip, const char *s, int len);

#endif /* NODE_H */
