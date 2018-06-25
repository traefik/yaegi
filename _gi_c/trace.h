#ifndef TRACE_H
#define TRACE_H

#include <stdio.h>

/* Macros for temporary internal debug traces */
#define _tr(fmt, type, exp)  fprintf(stderr, "[%s:%d %s] %s: " fmt, __FILE__,\
				     __LINE__, __func__, #exp, (type)(exp))
#define t_c(s)	_tr("'%c'\n", char, s)
#define t_d(s)	_tr("%ld\n", long int, s)
#define t_g(s)	_tr("%g\n", double, s)
#define t_i(s)	_tr("%d\n", int, s)
#define t_p(s)	_tr("%p\n", void *, s)
#define t_s(s)	_tr("'%s'\n", char *, s)
#define t_u(s)	_tr("%lu\n", unsigned long int, s)
#define t_x(s)	_tr("0x%lx\n", unsigned long int, s)
#define t_v(exp)  {fprintf(stderr, "[%s:%d %s] %s\n", __FILE__, __LINE__, __func__, #exp); (void)(exp);}

#define t_sn(s, n) do { \
	fprintf(stderr, "[%s:%d %s] %s: '", __FILE__, __LINE__, __func__, #s); \
	fwrite((s), (n), 1, stderr); fputs("'\n", stderr); } while (0)

#endif /* TRACE_H */
