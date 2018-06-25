#ifndef INIT_H
#define INIT_H

#include "node.h"

bip_t *bip_init(int trace_opt);
node_t *eval_str(bip_t *ip, node_t *node, char *s, int len);

#endif /* INIT_H */
