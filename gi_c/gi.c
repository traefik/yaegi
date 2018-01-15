/* a go interpreter */

#define VERSION "gi-c-0.1"

static const char man_txt[] =
"NAME\n"
"  gi - go interpreter\n"
"SYNOPSIS\n"
"  gi [-acnsVx] [-A ast_file] [-C cfg_file] [--] [script_file [args ...]]\n"
"DESCRIPTION\n"
"  gi is an interpreter that executes commands read from a file or the\n"
"  standard input.\n"
"OPTIONS\n"
"  -A ast_file  \n"
"	Write the abstract syntax tree in dot(1) format to ast_file or\n"
"	standard output if ast_file is -\n"
"  -a	Display AST graph using dotty(1)\n"
"  -C cfg_file  \n"
"	Write the control flow graph in dot(1) format to cfg_file or\n"
"	standard output if cfg_file is -\n"
"  -c	Display CFG graph using dotty(1)\n"
"  -n	Compile only, do not run\n"
"  -x   Generate and execute machine code using a JIT compiler\n"
"  -v   Trace each instruction during execution\n"
"  -V	Print interpreter version and exit\n"
"BIP LANGUAGE\n"
"  Commands are read in terms of lines of words separated by whitespaces\n"
"  (blanks or tabs) and certain sequences of characters called ``operators''.\n"
"  commands may also be separated by ';' or grouped between braces into a\n"
"  list\n"
"  OPERATORS\n"
"  Bip uses the usual infix notation for operators (example: c = a + b).\n"
"  Precedence rules identical to C are applied. Parenthesis are used to\n"
"  group expression and make precedence explicit.\n"
"  + - * / %  \n"	
"  	Arithmetic: addition, substraction, multiplication, division\n"
"	and modulo.\n"
"  < <= > >= == !=  \n"
"	Comparison: lower, lower or equal, greater, greater or equal,\n"
"	equal, not equal.\n"
"  ! && ||  \n"
"	Logical: not, and, or.\n"
"  =	Assignement.\n"
"  << >> & |  \n"
"	Binary: left shift, right shift, boolean and, boolean or.\n"
"  FLOW CONTROL\n"
"  if cond list1 [else list0]  \n"
"	the cond is executed and if it returns a non zero value, list1\n"
"	is executed. Otherwise, if defined, list0 is executed.\n"
"  while cond list  \n"
"	cond and list are repeatedly executed while cond returns non zero.\n"
"AUTHORS\n"
" MV\n"
;

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "init_go.h"
#include "trace.h"

#define MSIZE 4096

int source_stream(bip_t *ip, FILE *fd)
{
	int len = 0, max = 0;
	char *src = NULL;

	while (len == max) {	/* Grow src buffer until everything is read */
		max += MSIZE;
		src = (char *)realloc(src, max);
		len += fread(src + len, 1, MSIZE, fd);
	}
	src[len] = 0;
	eval_str(ip, NULL, src, len);
	printf("%s", ip->out);
	ip->out[0] = 0;
	free(src);
	return len;
}

int main(int argc, char *argv[], char *env[])
{
	int opt;
	bip_t *ip = bip_init(0);
	FILE *fd;
	
	while ((opt = getopt(argc, argv, "A:aC:chnpVvx")) != -1) {
		switch (opt) {
		case 'A':
			ip->fd_ast = strcmp(optarg, "-") ?
				 fopen(optarg, "w") : stdout;
			break;
		case 'a':
			ip->opt.a = 1;
			break;
		case 'C':
			ip->fd_cfg = strcmp(optarg, "-") ?
				 fopen(optarg, "w") : stdout;
			break;
		case 'c':
			ip->opt.c = 1;
			break;
		case 'h':
			printf("%s", man_txt);
			exit(0);
		case 'n':
			ip->opt.n = 1;
			break;
		case 'p':
			ip->opt.p = 1;
			break;
		case 'v':
			ip->opt.v = 1;
			break;
		case 'V':
			printf("%s\n", VERSION);
			exit(0);
		case 'x':
			ip->opt.x = 1;
			break;
		default:
			fprintf(stderr, "Usage: b1 [-acnVvx] [-A astname] "
				"[-C fgname] [file]\n");
			exit(1);
		}
	}
	fd = ((argc - optind) > 0) ? fopen(argv[optind], "r") : stdin;
	if (fd == NULL)
		return 1;
	while (source_stream(ip, fd));
	return 0;
}
