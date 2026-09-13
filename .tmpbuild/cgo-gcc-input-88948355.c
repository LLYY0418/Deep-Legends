
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
