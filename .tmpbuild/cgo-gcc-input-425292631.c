
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
#line 11 "/tmp/go/src/net/cgo_unix_cgo_res.go"

#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <netdb.h>
#include <unistd.h>
#include <string.h>
#include <arpa/nameser.h>
#include <resolv.h>



#line 1 "cgo-generated-wrapper"
#line 1 "not-declared"
void __cgo_f_1_1(void) { __typeof__(int) *__cgo_undefined__1; }
#line 1 "not-type"
void __cgo_f_1_2(void) { int *__cgo_undefined__2; }
#line 1 "not-int-const"
void __cgo_f_1_3(void) { enum { __cgo_undefined__3 = (int)*1 }; }
#line 1 "not-num-const"
void __cgo_f_1_4(void) { static const double __cgo_undefined__4 = (int); }
#line 1 "not-str-lit"
void __cgo_f_1_5(void) { static const char __cgo_undefined__5[] = (int); }
#line 2 "not-declared"
void __cgo_f_2_1(void) { __typeof__(res_search) *__cgo_undefined__1; }
#line 2 "not-type"
void __cgo_f_2_2(void) { res_search *__cgo_undefined__2; }
#line 2 "not-int-const"
void __cgo_f_2_3(void) { enum { __cgo_undefined__3 = (res_search)*1 }; }
#line 2 "not-num-const"
void __cgo_f_2_4(void) { static const double __cgo_undefined__4 = (res_search); }
#line 2 "not-str-lit"
void __cgo_f_2_5(void) { static const char __cgo_undefined__5[] = (res_search); }
#line 1 "completed"
int __cgo__1 = __cgo__2;
