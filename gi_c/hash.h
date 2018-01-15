#ifndef HASH_H
#define HASH_H

/* hash item */
typedef struct hitem_t {
	struct hitem_t *next;		/* next item in same slot */
	void *index;			/* pointer to index data */
	void *data;			/* pointer to value data */
	unsigned int key;		/* hash key, obtained from index */
} hitem_t;

/* key hash comparison function type */
typedef int (*cmpfun_t)(void *, void *, int);

/* dynamically resizable array / hash table */
typedef struct hash_t {
	hitem_t **htab;		/* hash slots */
	cmpfun_t cmpf;		/* user provided key comparison function */
	unsigned int len;	/* tab size */
	unsigned int nhash;	/* number of hashed items */
} hash_t;

/* hash table iterator */
#define hforeach(h, i, c) \
	for (c = 0; (unsigned int)c < (h)->len; c++) \
		for (i = (h)->htab[c]; i; i = i->next)

#define HNITEM_MAX	((unsigned)(1 << (8 * sizeof(int) -1)))	/* 2^31 on 32 bit */

hash_t *hinit(unsigned int n, cmpfun_t cmpf);
unsigned int hkey(char *s);
unsigned int hnkey(const char *s, int n, unsigned int h);
void *hadd(hash_t *h, unsigned int key, void *index, void *data);
void *hdel(hash_t *h, hitem_t *item);
void *hlookup(hash_t *h, unsigned int key, void *data, int len);

#endif /* HASH_H */
