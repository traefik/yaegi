#ifndef RUN_CFG_H
#define RUN_CFG_H

#include "node.h"

void nop(node_t *n);
void assign(node_t *n);
void echo(node_t *n);
void cond_branch(node_t *n);

void inc(node_t *n);
void add(node_t *n);
void sub(node_t *n);
void mul(node_t *n);
void fdiv(node_t *n);
void band(node_t *n);
void bor(node_t *n);
void lshift(node_t *n);
void rshift(node_t *n);
void mod(node_t *n);
void eq(node_t *n);
void neq(node_t *n);
void ge(node_t *n);
void gt(node_t *n);
void le(node_t *n);
void lt(node_t *n);

void run_cfg(bip_t *ip);

#endif /* RUN_CFG_H */
