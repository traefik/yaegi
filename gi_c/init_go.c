#include <stdlib.h>
#include <string.h>

#include "graph.h"
#include "node.h"
#include "parse_go.h"
#include "run_cfg.h"
#include "run_jit.h"
#include "scan.h"

#define X4_scan_bad	scan_bad, scan_bad, scan_bad, scan_bad
#define X16_scan_bad	X4_scan_bad, X4_scan_bad, X4_scan_bad, X4_scan_bad
#define X64_scan_bad	X16_scan_bad, X16_scan_bad, X16_scan_bad, X16_scan_bad

/* BIP language lexical table */

static const scan_fun_t scanfun[256] = {
	scan_bad, scan_bad, scan_bad, scan_bad, scan_bad,      // NUL SOH STX ETX EOT
	scan_bad, scan_bad, scan_bad, scan_bad, scan_wsep,     // ENQ ACQ BELL BS HT
	scan_csep, scan_bad, scan_bad, scan_csep, scan_bad,    // LF VT FF CR SO
	scan_bad, scan_bad, scan_bad, scan_bad, scan_bad,      // SI DLE DC1 DC2 DC3
	scan_bad, scan_bad, scan_bad, scan_bad, scan_bad,      // DC4 NAK SYN ETB CAN
	scan_bad, scan_bad, scan_bad, scan_bad, scan_bad,      // EM SUB ESC FS GS
	scan_bad, scan_bad, scan_wsep, scan_go_op, scan_str,    // RS US ' ' ! "
	scan_cmt, scan_go_op, scan_go_op, scan_go_op, scan_go_op,  // # $ % & '
	scan_paren, scan_bad, scan_go_op, scan_go_op, scan_lsep, // ( ) * + ,
	scan_go_op, scan_go_op, scan_go_op, scan_num, scan_num,   // - . / 0 1
	scan_num, scan_num, scan_num, scan_num, scan_num,      // 2 3 4 5 6
	scan_num, scan_num, scan_num, scan_go_op, scan_csep,    // 7 8 9 : ;
	scan_go_op, scan_go_op, scan_go_op, scan_go_op, scan_go_op, // < = > ? @
	scan_id, scan_id, scan_id, scan_id, scan_id, 	       // A B C D E
	scan_id, scan_id, scan_id, scan_id, scan_id,           // F G H I J
	scan_id, scan_id, scan_id, scan_id, scan_id,           // K L M N O
	scan_id, scan_id, scan_id, scan_id, scan_id,           // P Q R S T
	scan_id, scan_id, scan_id, scan_id, scan_id,           // U V W X Y
	scan_id, scan_bracket, scan_bad, scan_bad, scan_go_op,  // Z [ \ ] ^
	scan_id, scan_go_op, scan_id, scan_id, scan_id,         // _ ` a b c
	scan_id, scan_id, scan_id, scan_id, scan_id,           // d e f g h
	scan_id, scan_id, scan_id, scan_id, scan_id,           // i j k l m
	scan_id, scan_id, scan_id, scan_id, scan_id,           // n o p q r
	scan_id, scan_id, scan_id, scan_id, scan_id,           // s t u v w
	scan_id, scan_id, scan_id, scan_brace, scan_go_op,      // x y z { |
	scan_bad, scan_go_op, scan_bad,                         // } ~ DEL
	X64_scan_bad, X64_scan_bad                             // Non ASCII
};

static int cmpsym(void *data, void *sym, int n)
{
	return strncmp((char *)data, ((sym_t *)sym)->name, n);
}

bip_t *bip_init(int trace_opt)
{
	bip_t *ip = (bip_t *)calloc(1, sizeof(*ip));

	ip->scanfun = (scan_fun_t *)scanfun;
	ip->gsym = hinit(64, cmpsym);
	ip->lsym = hinit(16, cmpsym);
	newsym(ip->gsym, "if", IF, 0, nop, j_nop);
	newsym(ip->gsym, "break", BREAK, 0, NULL, j_nop);
	newsym(ip->gsym, "continue", CONTINUE, 0, NULL, j_nop);
	newsym(ip->gsym, "for", FOR, 0, nop, j_nop);
	newsym(ip->gsym, "func", DEF, 0, NULL, j_nop);
	newsym(ip->gsym, "return", RETURN, 0, NULL, j_nop);
	newsym(ip->gsym, "local", LOCAL, 0, NULL, j_nop);
	newsym(ip->gsym, "eval", FUN, 0, NULL, j_nop);
	newsym(ip->gsym, "print", FUN, 0, NULL, j_nop);
	newsym(ip->gsym, "println", FUN, 0, echo, j_echo);
	newsym(ip->gsym, "source", FUN, 0, NULL, j_nop);
	newsym(ip->gsym, "dsym", FUN, 0, NULL, j_nop);
	newsym(ip->gsym, "map", FUN, 0, NULL, j_nop);
	newsym(ip->gsym, "nop", OP, 0, nop, j_nop);
	newsym(ip->gsym, "SL", SL, 0, nop, j_nop);
	newsym(ip->gsym, "CB", COND_BRANCH, 0, cond_branch, j_nop);
	newsym(ip->gsym, "package", NAMESPACE, 0, nop, j_nop);
	newsym(ip->gsym, "++", OPS, 0, inc, j_inc);
	newsym(ip->gsym, "--", OPS, 0, NULL, j_nop);
	newsym(ip->gsym, "!", OP, 10, NULL, j_not);
	newsym(ip->gsym, "+", OP, 8, add, j_add);
	newsym(ip->gsym, "-", OP, 8, sub, j_sub);
	newsym(ip->gsym, "~", OP, 10, NULL, j_nop);
	newsym(ip->gsym, "^", OP, 10, NULL, j_xor);
	newsym(ip->gsym, "*", OP, 9, mul, j_mul);
	newsym(ip->gsym, "/", OP, 9, fdiv, j_div);
	newsym(ip->gsym, "%", OP, 9, mod, j_rem);
	newsym(ip->gsym, "<", OP, 6, lt, j_lt);
	newsym(ip->gsym, "<=", OP, 6, le, j_le);
	newsym(ip->gsym, ">=", OP, 6, ge, j_ge);
	newsym(ip->gsym, ">", OP, 6, gt, j_gt);
	newsym(ip->gsym, "==", OP, 5, eq, j_eq);
	newsym(ip->gsym, "!=", OP, 5, neq, j_ne);
	newsym(ip->gsym, ":=", OP, 0, assign, j_assign);
	newsym(ip->gsym, "=", OP, 0, assign, j_assign);
	newsym(ip->gsym, "&&", OP, 2, NULL, j_land);
	newsym(ip->gsym, "||", OP, 1, NULL, j_lor);
	newsym(ip->gsym, "&", OP, 4, band, j_and);
	newsym(ip->gsym, "|", OP, 3, bor, j_or);
	newsym(ip->gsym, "<<", OP, 7, lshift, j_lsh);
	newsym(ip->gsym, ">>", OP, 7, rshift, j_rsh);
	ip->out = (char *)calloc(1, 1024);
	ip->opt.x = trace_opt;
	return ip;
}

node_t *eval_str(bip_t *ip, node_t *node, char *s, int len)
{
	node_t *n = parse_sl(ip, &s, &len);

	if (n->nchild == 0)
		return node;    
	if (ip->fd_ast)
		print_tree(ip, ip->fd_ast, n);
	if (ip->opt.a)
		dotty(ip, n, print_tree);
	n = n->child[1]->child[2];
	ast2cfg(ip, n);
	if (ip->fd_cfg)
		print_flow(ip, ip->fd_cfg, n);
	if (ip->opt.c)
		dotty(ip, n, print_flow);
	if (ip->opt.x)
		run_jit(ip, n);
	else if (!ip->opt.n) {
		run_cfg(ip);
	}
	return node ? node : n;
}

void bip_eval(bip_t *ip, char *s, char **res)
{
	eval_str(ip, NULL, s, strlen(s));
	*res = ip->out;
}
