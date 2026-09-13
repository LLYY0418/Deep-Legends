
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
#line 1 "not-declared"
void __cgo_f_1_1(void) { __typeof__(GoString) *__cgo_undefined__1; }
#line 1 "not-type"
void __cgo_f_1_2(void) { GoString *__cgo_undefined__2; }
#line 1 "not-int-const"
void __cgo_f_1_3(void) { enum { __cgo_undefined__3 = (GoString)*1 }; }
#line 1 "not-num-const"
void __cgo_f_1_4(void) { static const double __cgo_undefined__4 = (GoString); }
#line 1 "not-str-lit"
void __cgo_f_1_5(void) { static const char __cgo_undefined__5[] = (GoString); }
#line 2 "not-declared"
void __cgo_f_2_1(void) { __typeof__(_CMalloc) *__cgo_undefined__1; }
#line 2 "not-type"
void __cgo_f_2_2(void) { _CMalloc *__cgo_undefined__2; }
#line 2 "not-int-const"
void __cgo_f_2_3(void) { enum { __cgo_undefined__3 = (_CMalloc)*1 }; }
#line 2 "not-num-const"
void __cgo_f_2_4(void) { static const double __cgo_undefined__4 = (_CMalloc); }
#line 2 "not-str-lit"
void __cgo_f_2_5(void) { static const char __cgo_undefined__5[] = (_CMalloc); }
#line 3 "not-declared"
void __cgo_f_3_1(void) { __typeof__(_GoString_) *__cgo_undefined__1; }
#line 3 "not-type"
void __cgo_f_3_2(void) { _GoString_ *__cgo_undefined__2; }
#line 3 "not-int-const"
void __cgo_f_3_3(void) { enum { __cgo_undefined__3 = (_GoString_)*1 }; }
#line 3 "not-num-const"
void __cgo_f_3_4(void) { static const double __cgo_undefined__4 = (_GoString_); }
#line 3 "not-str-lit"
void __cgo_f_3_5(void) { static const char __cgo_undefined__5[] = (_GoString_); }
#line 4 "not-declared"
void __cgo_f_4_1(void) { __typeof__(__socklen_t) *__cgo_undefined__1; }
#line 4 "not-type"
void __cgo_f_4_2(void) { __socklen_t *__cgo_undefined__2; }
#line 4 "not-int-const"
void __cgo_f_4_3(void) { enum { __cgo_undefined__3 = (__socklen_t)*1 }; }
#line 4 "not-num-const"
void __cgo_f_4_4(void) { static const double __cgo_undefined__4 = (__socklen_t); }
#line 4 "not-str-lit"
void __cgo_f_4_5(void) { static const char __cgo_undefined__5[] = (__socklen_t); }
#line 5 "not-declared"
void __cgo_f_5_1(void) { __typeof__(char) *__cgo_undefined__1; }
#line 5 "not-type"
void __cgo_f_5_2(void) { char *__cgo_undefined__2; }
#line 5 "not-int-const"
void __cgo_f_5_3(void) { enum { __cgo_undefined__3 = (char)*1 }; }
#line 5 "not-num-const"
void __cgo_f_5_4(void) { static const double __cgo_undefined__4 = (char); }
#line 5 "not-str-lit"
void __cgo_f_5_5(void) { static const char __cgo_undefined__5[] = (char); }
#line 6 "not-declared"
void __cgo_f_6_1(void) { __typeof__(free) *__cgo_undefined__1; }
#line 6 "not-type"
void __cgo_f_6_2(void) { free *__cgo_undefined__2; }
#line 6 "not-int-const"
void __cgo_f_6_3(void) { enum { __cgo_undefined__3 = (free)*1 }; }
#line 6 "not-num-const"
void __cgo_f_6_4(void) { static const double __cgo_undefined__4 = (free); }
#line 6 "not-str-lit"
void __cgo_f_6_5(void) { static const char __cgo_undefined__5[] = (free); }
#line 7 "not-declared"
void __cgo_f_7_1(void) { __typeof__(freeaddrinfo) *__cgo_undefined__1; }
#line 7 "not-type"
void __cgo_f_7_2(void) { freeaddrinfo *__cgo_undefined__2; }
#line 7 "not-int-const"
void __cgo_f_7_3(void) { enum { __cgo_undefined__3 = (freeaddrinfo)*1 }; }
#line 7 "not-num-const"
void __cgo_f_7_4(void) { static const double __cgo_undefined__4 = (freeaddrinfo); }
#line 7 "not-str-lit"
void __cgo_f_7_5(void) { static const char __cgo_undefined__5[] = (freeaddrinfo); }
#line 8 "not-declared"
void __cgo_f_8_1(void) { __typeof__(gai_strerror) *__cgo_undefined__1; }
#line 8 "not-type"
void __cgo_f_8_2(void) { gai_strerror *__cgo_undefined__2; }
#line 8 "not-int-const"
void __cgo_f_8_3(void) { enum { __cgo_undefined__3 = (gai_strerror)*1 }; }
#line 8 "not-num-const"
void __cgo_f_8_4(void) { static const double __cgo_undefined__4 = (gai_strerror); }
#line 8 "not-str-lit"
void __cgo_f_8_5(void) { static const char __cgo_undefined__5[] = (gai_strerror); }
#line 9 "not-declared"
void __cgo_f_9_1(void) { __typeof__(getaddrinfo) *__cgo_undefined__1; }
#line 9 "not-type"
void __cgo_f_9_2(void) { getaddrinfo *__cgo_undefined__2; }
#line 9 "not-int-const"
void __cgo_f_9_3(void) { enum { __cgo_undefined__3 = (getaddrinfo)*1 }; }
#line 9 "not-num-const"
void __cgo_f_9_4(void) { static const double __cgo_undefined__4 = (getaddrinfo); }
#line 9 "not-str-lit"
void __cgo_f_9_5(void) { static const char __cgo_undefined__5[] = (getaddrinfo); }
#line 10 "not-declared"
void __cgo_f_10_1(void) { __typeof__(int) *__cgo_undefined__1; }
#line 10 "not-type"
void __cgo_f_10_2(void) { int *__cgo_undefined__2; }
#line 10 "not-int-const"
void __cgo_f_10_3(void) { enum { __cgo_undefined__3 = (int)*1 }; }
#line 10 "not-num-const"
void __cgo_f_10_4(void) { static const double __cgo_undefined__4 = (int); }
#line 10 "not-str-lit"
void __cgo_f_10_5(void) { static const char __cgo_undefined__5[] = (int); }
#line 11 "not-declared"
void __cgo_f_11_1(void) { __typeof__(intgo) *__cgo_undefined__1; }
#line 11 "not-type"
void __cgo_f_11_2(void) { intgo *__cgo_undefined__2; }
#line 11 "not-int-const"
void __cgo_f_11_3(void) { enum { __cgo_undefined__3 = (intgo)*1 }; }
#line 11 "not-num-const"
void __cgo_f_11_4(void) { static const double __cgo_undefined__4 = (intgo); }
#line 11 "not-str-lit"
void __cgo_f_11_5(void) { static const char __cgo_undefined__5[] = (intgo); }
#line 12 "not-declared"
void __cgo_f_12_1(void) { __typeof__(ptrdiff_t) *__cgo_undefined__1; }
#line 12 "not-type"
void __cgo_f_12_2(void) { ptrdiff_t *__cgo_undefined__2; }
#line 12 "not-int-const"
void __cgo_f_12_3(void) { enum { __cgo_undefined__3 = (ptrdiff_t)*1 }; }
#line 12 "not-num-const"
void __cgo_f_12_4(void) { static const double __cgo_undefined__4 = (ptrdiff_t); }
#line 12 "not-str-lit"
void __cgo_f_12_5(void) { static const char __cgo_undefined__5[] = (ptrdiff_t); }
#line 13 "not-declared"
void __cgo_f_13_1(void) { __typeof__(sa_family_t) *__cgo_undefined__1; }
#line 13 "not-type"
void __cgo_f_13_2(void) { sa_family_t *__cgo_undefined__2; }
#line 13 "not-int-const"
void __cgo_f_13_3(void) { enum { __cgo_undefined__3 = (sa_family_t)*1 }; }
#line 13 "not-num-const"
void __cgo_f_13_4(void) { static const double __cgo_undefined__4 = (sa_family_t); }
#line 13 "not-str-lit"
void __cgo_f_13_5(void) { static const char __cgo_undefined__5[] = (sa_family_t); }
#line 14 "not-declared"
void __cgo_f_14_1(void) { __typeof__(size_t) *__cgo_undefined__1; }
#line 14 "not-type"
void __cgo_f_14_2(void) { size_t *__cgo_undefined__2; }
#line 14 "not-int-const"
void __cgo_f_14_3(void) { enum { __cgo_undefined__3 = (size_t)*1 }; }
#line 14 "not-num-const"
void __cgo_f_14_4(void) { static const double __cgo_undefined__4 = (size_t); }
#line 14 "not-str-lit"
void __cgo_f_14_5(void) { static const char __cgo_undefined__5[] = (size_t); }
#line 15 "not-declared"
void __cgo_f_15_1(void) { __typeof__(socklen_t) *__cgo_undefined__1; }
#line 15 "not-type"
void __cgo_f_15_2(void) { socklen_t *__cgo_undefined__2; }
#line 15 "not-int-const"
void __cgo_f_15_3(void) { enum { __cgo_undefined__3 = (socklen_t)*1 }; }
#line 15 "not-num-const"
void __cgo_f_15_4(void) { static const double __cgo_undefined__4 = (socklen_t); }
#line 15 "not-str-lit"
void __cgo_f_15_5(void) { static const char __cgo_undefined__5[] = (socklen_t); }
#line 16 "not-declared"
void __cgo_f_16_1(void) { __typeof__(unsigned char) *__cgo_undefined__1; }
#line 16 "not-type"
void __cgo_f_16_2(void) { unsigned char *__cgo_undefined__2; }
#line 16 "not-int-const"
void __cgo_f_16_3(void) { enum { __cgo_undefined__3 = (unsigned char)*1 }; }
#line 16 "not-num-const"
void __cgo_f_16_4(void) { static const double __cgo_undefined__4 = (unsigned char); }
#line 16 "not-str-lit"
void __cgo_f_16_5(void) { static const char __cgo_undefined__5[] = (unsigned char); }
#line 17 "not-declared"
void __cgo_f_17_1(void) { __typeof__(unsigned int) *__cgo_undefined__1; }
#line 17 "not-type"
void __cgo_f_17_2(void) { unsigned int *__cgo_undefined__2; }
#line 17 "not-int-const"
void __cgo_f_17_3(void) { enum { __cgo_undefined__3 = (unsigned int)*1 }; }
#line 17 "not-num-const"
void __cgo_f_17_4(void) { static const double __cgo_undefined__4 = (unsigned int); }
#line 17 "not-str-lit"
void __cgo_f_17_5(void) { static const char __cgo_undefined__5[] = (unsigned int); }
#line 1 "completed"
int __cgo__1 = __cgo__2;
