#ifndef SCAN_H
#define SCAN_H

/* token types recognized by the scanner. For block operators, the
 * opening char is used.
 */
typedef enum scan_token_t {
	BAD, BRACE, BRACKET, BSTR, CSEP, FLOAT, ID, INT, LSEP, OPER, PAREN, STR,
	SCAN_TOKEN_LEN
} scan_token_t;

/* strings of above for debug */
extern const char *scan_token[SCAN_TOKEN_LEN];

/* scanner state */
typedef struct scan_t {
	char *tok;		/* token start position in input */
	char *end;		/* next position in input */
	char *orig;		/* previous position */
	scan_token_t type;	/* token type */
	int len;		/* token len */
	long num;		/* scanner returned value if type is INT */
	double fnum;		/* scanner returned value if type is FLOAT */
} scan_t;

typedef int (*scan_fun_t)(scan_t *ps, char *s, int len);

int scan_block(scan_t *ps, char *s, scan_token_t type, int len, char bstart, char bend, char sdelim);

int scan_paren(scan_t *ps, char *s, int len);
int scan_brace(scan_t *ps, char *s, int len);
int scan_bracket(scan_t *ps, char *s, int len);
int scan_cmt(scan_t *ps, char *s, int len);
int scan_num(scan_t *ps, char *s, int len);
int scan_id(scan_t *ps, char *s, int len);
int scan_wsep(scan_t *ps, char *s, int len);
int scan_lsep(scan_t *ps, char *s, int len);
int scan_csep(scan_t *ps, char *s, int len);
int scan_str(scan_t *ps, char *s, int len);
int scan_c_op(scan_t *ps, char *s, int len);
int scan_go_op(scan_t *ps, char *s, int len);
int scan_bad(scan_t *ps, char *s, int len);

int scan(scan_fun_t *scanfun, scan_t *ps, char **pstr, int *plen);
void unscan(scan_t *ps, char **pstr, int *plen);

#endif /* SCAN_H */
