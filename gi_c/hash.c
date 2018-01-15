#include <stdlib.h>

#include "hash.h"

hash_t *hinit(unsigned int n, cmpfun_t cmpf)
{
	hash_t *h = (hash_t *)malloc(sizeof(hash_t));
	h->htab = (hitem_t **)calloc(n, sizeof(hitem_t *));
	h->cmpf = cmpf;
	h->len = n;
	h->nhash = 0;
	return h;
}

static void hresize(hash_t *h, unsigned int n)
{
	unsigned int i, j;
	hitem_t *p, *pp, **tab = (hitem_t **)calloc(n, sizeof(hitem_t *));

	for (i = 0; i < h->len; i++) {
		p = h->htab[i];
		while (p) {
			pp = p->next;
			j = p->key % n;
			p->next = tab[j];
			tab[j] = p;
			p = pp;
		}
	}
	free(h->htab);
	h->htab = tab;
	h->len = n;
}

unsigned int hkey(char *s)
{
	unsigned int h = 0;
	while (*s)
		h = h * 31 + *s++;
	return h;
}

unsigned int hnkey(const char *s, int n, unsigned int h)
{
	while (n--)
		h = h * 31 + *s++;
	return h;
}

void *hadd(hash_t *h, unsigned int key, void *index, void *data)
{
	hitem_t *p;
	unsigned int i;

	if (h->nhash >= h->len && h->len < HNITEM_MAX)
		hresize(h, h->len * 2);
	p = (hitem_t *)malloc(sizeof(hitem_t));
	i = key < h->len ? key : key % h->len;
	p->key = key;
	p->data = data;
	p->index = index;
	p->next = h->htab[i];
	h->htab[i] = p;
	h->nhash++;
	return data;
}

void *hdel(hash_t *h, hitem_t *item)
{
	unsigned int i = item->key % h->len;
	hitem_t *p = h->htab[i];
	void *data = item->data;

	if (p == item) {
		h->htab[i] = p->next;
		goto del;
	}
	for (; p; p = p->next)
		if (p->next == item) {
			p->next = p->next->next;
			goto del;
		}
	return (void *)-1;
del:	h->nhash--;
	free(item);
	return data;
}

void *hlookup(hash_t *h, unsigned int key, void *data, int len)
{
	hitem_t *p;
	unsigned int i = key < h->len ? key : key % h->len;
	for (p = h->htab[i]; p; p = p->next) {
		if (p->key == key) {
			if (data && h->cmpf && (*h->cmpf)(data, p->data, len))
				continue;  /* key collision */
			else
				return p->data;
		}
	}
	return NULL;
}
