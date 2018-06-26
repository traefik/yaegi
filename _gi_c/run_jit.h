#ifndef RUN_JIT_H
#define RUN_JIT_H

#include "node.h"

void j_assign(node_t *n);
void j_echo(node_t *n);
void j_nop(node_t *n);

void j_inc(node_t *n);

void j_add(node_t *n);		/* c0 + c1 */
void j_sub(node_t *n);		/* c0 - c1 */
void j_mul(node_t *n);		/* c0 * c1 */
void j_div(node_t *n);		/* c0 / c1 */
void j_rem(node_t *n);		/* c0 % c1 */
void j_lt(node_t *n);		/* c0 < c1 */
void j_le(node_t *n);		/* c0 <= c1 */
void j_eq(node_t *n);		/* c0 == c1 */
void j_ne(node_t *n);		/* c0 != c1 */
void j_gt(node_t *n);		/* c0 > c1 */
void j_ge(node_t *n);		/* c0 >= c1 */
void j_and(node_t *n);		/* c0 & c1 */
void j_or(node_t *n);		/* c0 | c1 */
void j_xor(node_t *n);		/* c0 ^ c1 */
void j_lsh(node_t *n);		/* c0 << c1 */
void j_rsh(node_t *n);		/* c0 >> c1 */
void j_land(node_t *n);		/* c0 && c1 */
void j_lor(node_t *n);		/* c0 || c1 */
void j_neg(node_t *n);
void j_com(node_t *n);
void j_not(node_t *n);		/* ! c0 */

void run_jit(bip_t *ip, node_t *node);

#endif /* RUN_JIT_H */
