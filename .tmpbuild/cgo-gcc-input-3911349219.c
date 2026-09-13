
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
#line 9 "/tmp/go/src/net/cgo_socknew.go"

#include <sys/types.h>
#include <sys/socket.h>

#include <netinet/in.h>

#line 1 "cgo-generated-wrapper"
#line 1 "not-declared"
void __cgo_f_1_1(void) { __typeof__(sa_family_t) *__cgo_undefined__1; }
#line 1 "not-type"
void __cgo_f_1_2(void) { sa_family_t *__cgo_undefined__2; }
#line 1 "not-int-const"
void __cgo_f_1_3(void) { enum { __cgo_undefined__3 = (sa_family_t)*1 }; }
#line 1 "not-num-const"
void __cgo_f_1_4(void) { static const double __cgo_undefined__4 = (sa_family_t); }
#line 1 "not-str-lit"
void __cgo_f_1_5(void) { static const char __cgo_undefined__5[] = (sa_family_t); }
#line 1 "completed"
int __cgo__1 = __cgo__2;
