
#line 1 "cgo-builtin-prolog"
#include <stddef.h>

/* Define intgo when compiling with GCC.  */
typedef ptrdiff_t intgo;

#define GO_CGO_GOSTRING_TYPEDEF
typedef struct { const char *p; intgo n; } _GoString_;
typedef struct { char *p; intgo n; intgo c; } _GoBytes_;
_GoString_ GoString(char *p);
_GoString_ GoStringN(char *p, int l);
_GoBytes_ GoBytes(void *p, int n);
char *CString(_GoString_);
void *CBytes(_GoBytes_);
void *_CMalloc(size_t);

__attribute__ ((unused))
static size_t _GoStringLen(_GoString_ s) { return (size_t)s.n; }

__attribute__ ((unused))
static const char *_GoStringPtr(_GoString_ s) { return s.p; }
#line 9 "/tmp/go/src/net/cgo_unix_cgo.go"

#define _GNU_SOURCE 1


#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <netdb.h>
#include <unistd.h>
#include <string.h>
#include <stdlib.h>

#ifndef EAI_NODATA
#define EAI_NODATA -5
#endif

// If nothing else defined EAI_ADDRFAMILY, make sure it has a value.
#ifndef EAI_ADDRFAMILY
#define EAI_ADDRFAMILY -9
#endif

// If nothing else defined EAI_OVERFLOW, make sure it has a value.
#ifndef EAI_OVERFLOW
#define EAI_OVERFLOW -12
#endif

#line 1 "cgo-generated-wrapper"
#line 1 "cgo-dwarf-inference"
__typeof__(struct addrinfo) *__cgo__0;
__typeof__(struct sockaddr) *__cgo__1;
__typeof__(GoString) *__cgo__2;
__typeof__(_CMalloc) *__cgo__3;
__typeof__(_GoString_) *__cgo__4;
__typeof__(__socklen_t) *__cgo__5;
__typeof__(char) *__cgo__6;
__typeof__(free) *__cgo__7;
__typeof__(freeaddrinfo) *__cgo__8;
__typeof__(gai_strerror) *__cgo__9;
__typeof__(getaddrinfo) *__cgo__10;
__typeof__(int) *__cgo__11;
__typeof__(intgo) *__cgo__12;
__typeof__(ptrdiff_t) *__cgo__13;
__typeof__(sa_family_t) *__cgo__14;
__typeof__(size_t) *__cgo__15;
__typeof__(socklen_t) *__cgo__16;
__typeof__(unsigned char) *__cgo__17;
__typeof__(unsigned int) *__cgo__18;
long long __cgodebug_ints[] = {
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	1
};
double __cgodebug_floats[] = {
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	1
};
