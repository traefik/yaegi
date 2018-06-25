/* scanner: lexical analysis functions */

/*
 * Todo:
 *  - line counting
 *  - better error reporting
 */
#include <stdlib.h>

#include "scan.h"
#include "trace.h"

/* scan_token strings for debug. Keep in sync with scan_token_t in scan.h */
const char *scan_token[SCAN_TOKEN_LEN] = {
	"BAD", "BRACE", "BRACKET", "BSTR", "CSEP", "FLOAT", "ID", "INT", "LSEP",
	"OPER", "PAREN", "STR"
};

#define isdigit(c)	('0' <= (c) && (c) <= '9')
#define isalpha(c)	(('a' <= (c) && (c) <= 'z') || ('A' <= (c) && (c) <= 'Z'))
#define isalnum(c)	(isdigit((c)) || isalpha((c)))

/* Return a pointer on the first occurence of char c in string s of length len */
static char *strnchr(char *s, int len, char c)
{
	int i;
	for (i = 0; i < len; i++)
		if (s[i] == c)
			return &s[i];
	return NULL;
}

/* Flat scan of block delimited by i.e (), [], or {} */
int scan_block(scan_t *ps, char *s, scan_token_t type, int len, char bstart, char bend, char sdelim)
{
	char c;
	int complete = 0, inquote = 0, level = 0;

	ps->tok = ++s;
	c = *s;
	while (len--) {
		if (c == sdelim)
			inquote = 1 - inquote;
		else if (inquote && c == '\\')
			s++;
		else if (!inquote && c == bstart)
			level++;
		else if (!inquote && c == bend) {
			if (--level < 0) {
				complete = 1;
				break;
			}
		}
		c = *++s;
	}
	ps->type = complete ? type : BAD;
	ps->end = s + 1;
	ps->len = ps->end - ps->tok -1;
	return 0;
}

int scan_paren(scan_t *ps, char *s, int len)
{
	return scan_block(ps, s, PAREN, len, '(', ')', '"');
}

int scan_brace(scan_t *ps, char *s, int len)
{
	return scan_block(ps, s, BRACE, len, '{', '}', '"');
}

int scan_bracket(scan_t *ps, char *s, int len)
{
	return scan_block(ps, s, BRACKET, len, '[', ']', '"');
}

int scan_cmt(scan_t *ps, char *s, int len)
{
	char c, *smax = s + len;
	for (c = *++s; s < smax && c != '\n'; c = *++s);
	ps->end = s;
	return 1;
}

int scan_num(scan_t *ps, char *s, int len)
{
	ps->fnum = strtod(s, &ps->end);
	ps->tok = s;
	ps->len = ps->end - ps->tok;
	if (strnchr(ps->tok, ps->len, '.'))
		ps->type = FLOAT;
	else {
		if (ps->tok[0] == '0' && ps->tok[1] != 'x')
			ps->num = strtol(ps->tok, &ps->end, 8);
		else
			ps->num = (long)ps->fnum;
		ps->type = INT;
	}
	return 0;
}

int scan_id(scan_t *ps, char *s, int len)
{
	char c, *smax = s + len;
	ps->tok = s;
	ps->type = ID;
	for (c = *++s; s < smax; c = *++s)
		if (!(isalnum(c) || c == '_'))	/* XXX could be accelerated */
			break;
	ps->end = s;
	ps->len = s - ps->tok;
	return 0;
}

int scan_wsep(scan_t *ps, char *s, int len)
{
	char c, *smax = s + len;
	for (c = *++s; s < smax; c = *++s) {
		if (c != ' ' && c != '\t')
			break;
	}
	ps->end = s;
	return 1;
}

int scan_lsep(scan_t *ps, char *s, int len)
{
	ps->type = LSEP;
	ps->end = s + 1;
	return 0;
}

int scan_csep(scan_t *ps, char *s, int len)
{
	char c = *s, *smax = s + len;

	for (c = *++s; s < smax; c = *++s)
		if (c != '\n' && c != ';' && c != ' ' && c != '\t')
			break;
	ps->type = CSEP;
	ps->end = s;
	return 0;
}

int scan_str(scan_t *ps, char *s, int len)
{
	char *smax = s + len, c, delim = *s;

	ps->type = STR;
	ps->tok = ++s;
	for (c = *ps->tok; s < smax; c = *++s)
		if (c == '\\')
			s++, ps->type = BSTR;
		else if (c == delim)
			break;
	if (s == smax && c != delim)
		ps->type = BAD;
	ps->len = s - ps->tok;
	ps->end = s + 1;
	return 0;
}

int scan_c_op(scan_t *ps, char *s, int len)
{
	ps->tok = s;
	ps->type = OPER;
	switch (*s) {
	case '!': if (s[1] == '=') s++; break;
	case '=': if (s[1] == '=') s++; break;
	case '<': if (s[1] == '=' || s[1] == '<') s++; break;
	case '>': if (s[1] == '=' || s[1] == '>') s++; break;
	case '&': if (s[1] == '&') s++; break;
	case '|': if (s[1] == '|') s++; break;
	case '+': if (s[1] == '+') s++; break;
	case '-': if (s[1] == '-') s++; break;
	}
	ps->end = s + 1;
	ps->len = ps->end - ps->tok;
	return 0;
}

int scan_go_op(scan_t *ps, char *s, int len)
{
	ps->tok = s;
	ps->type = OPER;
	switch (*s) {
	case ':': if (s[1] == '=') s++; break;
	case '!': if (s[1] == '=') s++; break;
	case '=': if (s[1] == '=') s++; break;
	case '<': if (s[1] == '=' || s[1] == '<') s++; break;
	case '>': if (s[1] == '=' || s[1] == '>') s++; break;
	case '&': if (s[1] == '&') s++; break;
	case '|': if (s[1] == '|') s++; break;
	case '+': if (s[1] == '+') s++; break;
	case '-': if (s[1] == '-') s++; break;
	}
	ps->end = s + 1;
	ps->len = ps->end - ps->tok;
	return 0;
}

/* return a bad token */
int scan_bad(scan_t *ps, char *s, int len)
{
	t_c(*s);
	ps->tok = s;
	ps->end = s + 1;
	ps->type = BAD;
	return 0;
}

/* scanner entry point */
int scan(scan_fun_t *scanfun, scan_t *ps, char **pstr, int *plen)
{
	char *s = *pstr, *smax = s + *plen;
 
 	ps->orig = s;
	ps->len = 0;
	ps->type = BAD;
	while (s < smax && (*scanfun[(int)*s])(ps, s, *plen))
		s = ps->end;
	*plen += *pstr - ps->end;
	*pstr = ps->end;
	return ps->type;
}

/* undo scan operation, revert to previous state */
void unscan(scan_t *ps, char **pstr, int *plen)
{
	*pstr = ps->orig;
	*plen += ps->end - ps->orig;
}
