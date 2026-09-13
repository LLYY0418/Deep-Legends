	.arch armv8-a
	.file	"gcc_linux_arm64.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_linux_arm64.c"
	.align	2
	.p2align 4,,11
	.type	threadentry, %function
threadentry:
.LVL0:
.LFB53:
	.file 1 "gcc_linux_arm64.c"
	.loc 1 47 1 view -0
	.cfi_startproc
	.loc 1 48 2 view .LVU1
	.loc 1 50 2 view .LVU2
	.loc 1 47 1 is_stmt 0 view .LVU3
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -16
	.cfi_offset 20, -8
	.loc 1 50 5 view .LVU4
	ldr	x20, [x0]
.LVL1:
	.loc 1 50 5 view .LVU5
	ldr	x19, [x0, 16]
.LVL2:
	.loc 1 51 2 is_stmt 1 view .LVU6
	bl	free
.LVL3:
	.loc 1 53 2 view .LVU7
	adrp	x1, .LANCHOR0
	mov	x2, x20
	mov	x0, x19
	ldr	x1, [x1, #:lo12:.LANCHOR0]
	bl	crosscall1
.LVL4:
	.loc 1 54 2 view .LVU8
	.loc 1 55 1 is_stmt 0 view .LVU9
	mov	x0, 0
	ldp	x19, x20, [sp, 16]
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE53:
	.size	threadentry, .-threadentry
	.section	.rodata.str1.8,"aMS",@progbits,1
	.align	3
.LC0:
	.string	"pthread_create failed: %s"
	.text
	.align	2
	.p2align 4,,11
	.global	_cgo_sys_thread_start
	.type	_cgo_sys_thread_start, %function
_cgo_sys_thread_start:
.LVL5:
.LFB52:
	.loc 1 20 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 21 2 view .LVU11
	.loc 1 22 2 view .LVU12
	.loc 1 23 2 view .LVU13
	.loc 1 24 2 view .LVU14
	.loc 1 25 2 view .LVU15
	.loc 1 27 2 view .LVU16
	.loc 1 20 1 is_stmt 0 view .LVU17
	stp	x29, x30, [sp, -384]!
	.cfi_def_cfa_offset 384
	.cfi_offset 29, -384
	.cfi_offset 30, -376
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -368
	.cfi_offset 20, -360
	.loc 1 27 2 view .LVU18
	add	x19, sp, 128
	.loc 1 28 2 view .LVU19
	add	x20, sp, 256
	.loc 1 20 1 view .LVU20
	str	x21, [sp, 32]
	.cfi_offset 21, -352
	.loc 1 20 1 view .LVU21
	mov	x21, x0
	.loc 1 27 2 view .LVU22
	mov	x0, x19
.LVL6:
	.loc 1 27 2 view .LVU23
	bl	sigfillset
.LVL7:
	.loc 1 28 2 is_stmt 1 view .LVU24
	mov	x2, x20
	mov	x1, x19
	mov	w0, 2
	.loc 1 30 2 is_stmt 0 view .LVU25
	add	x19, sp, 64
	.loc 1 28 2 view .LVU26
	bl	pthread_sigmask
.LVL8:
	.loc 1 30 2 is_stmt 1 view .LVU27
	mov	x0, x19
	bl	pthread_attr_init
.LVL9:
	.loc 1 31 2 view .LVU28
	mov	x0, x19
	mov	w1, 1
	bl	pthread_attr_setdetachstate
.LVL10:
	.loc 1 32 2 view .LVU29
	mov	x0, x19
	add	x1, sp, 56
	bl	pthread_attr_getstacksize
.LVL11:
	.loc 1 34 2 view .LVU30
	.loc 1 34 17 is_stmt 0 view .LVU31
	ldr	x0, [x21]
	.loc 1 35 8 view .LVU32
	mov	x3, x21
	.loc 1 34 17 view .LVU33
	ldr	x4, [sp, 56]
	.loc 1 35 8 view .LVU34
	mov	x1, x19
	.loc 1 34 17 view .LVU35
	str	x4, [x0, 8]
	.loc 1 35 2 is_stmt 1 view .LVU36
	.loc 1 35 8 is_stmt 0 view .LVU37
	adrp	x2, threadentry
	add	x0, sp, 48
	add	x2, x2, :lo12:threadentry
	bl	_cgo_try_pthread_create
.LVL12:
	mov	w19, w0
.LVL13:
	.loc 1 37 2 is_stmt 1 view .LVU38
	mov	x1, x20
	mov	x2, 0
	mov	w0, 2
.LVL14:
	.loc 1 37 2 is_stmt 0 view .LVU39
	bl	pthread_sigmask
.LVL15:
	.loc 1 39 2 is_stmt 1 view .LVU40
	.loc 1 39 5 is_stmt 0 view .LVU41
	cbnz	w19, .L7
	.loc 1 42 1 view .LVU42
	ldp	x19, x20, [sp, 16]
.LVL16:
	.loc 1 42 1 view .LVU43
	ldr	x21, [sp, 32]
.LVL17:
	.loc 1 42 1 view .LVU44
	ldp	x29, x30, [sp], 384
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL18:
.L7:
	.cfi_restore_state
	.loc 1 40 3 is_stmt 1 view .LVU45
	mov	w0, w19
	bl	strerror
.LVL19:
	adrp	x2, .LC0
	mov	x1, x0
	add	x0, x2, :lo12:.LC0
	bl	fatalf
.LVL20:
	.cfi_endproc
.LFE52:
	.size	_cgo_sys_thread_start, .-_cgo_sys_thread_start
	.section	.rodata.str1.8
	.align	3
.LC1:
	.string	"malloc failed: %s"
	.text
	.align	2
	.p2align 4,,11
	.global	x_cgo_init
	.type	x_cgo_init, %function
x_cgo_init:
.LVL21:
.LFB54:
	.loc 1 59 1 view -0
	.cfi_startproc
	.loc 1 60 2 view .LVU47
	.loc 1 77 2 view .LVU48
	.loc 1 59 1 is_stmt 0 view .LVU49
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	.loc 1 77 11 view .LVU50
	adrp	x4, .LANCHOR0
	.loc 1 59 1 view .LVU51
	mov	x29, sp
	.loc 1 77 11 view .LVU52
	str	x1, [x4, #:lo12:.LANCHOR0]
	.loc 1 78 2 is_stmt 1 view .LVU53
	.loc 1 59 1 is_stmt 0 view .LVU54
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	mov	x20, x0
	.loc 1 78 22 view .LVU55
	mov	x0, 16
.LVL22:
	.loc 1 59 1 view .LVU56
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -16
	.cfi_offset 22, -8
	.loc 1 59 1 view .LVU57
	mov	x21, x2
	mov	x22, x3
	.loc 1 78 22 view .LVU58
	bl	malloc
.LVL23:
	.loc 1 79 2 is_stmt 1 view .LVU59
	.loc 1 79 5 is_stmt 0 view .LVU60
	cbz	x0, .L12
	.loc 1 82 2 is_stmt 1 view .LVU61
	mov	x1, x0
	mov	x19, x0
	mov	x0, x20
.LVL24:
	.loc 1 82 2 is_stmt 0 view .LVU62
	bl	_cgo_set_stacklo
.LVL25:
	.loc 1 83 2 is_stmt 1 view .LVU63
	mov	x0, x19
	bl	free
.LVL26:
	.loc 1 85 2 view .LVU64
	.loc 1 85 6 is_stmt 0 view .LVU65
	adrp	x4, :got:x_cgo_inittls
	ldr	x4, [x4, #:got_lo12:x_cgo_inittls]
	ldr	x2, [x4]
	.loc 1 85 5 view .LVU66
	cbz	x2, .L8
	.loc 1 86 3 is_stmt 1 view .LVU67
	mov	x1, x22
	mov	x0, x21
	.loc 1 88 1 is_stmt 0 view .LVU68
	ldp	x19, x20, [sp, 16]
.LVL27:
	.loc 1 86 3 view .LVU69
	mov	x16, x2
	.loc 1 88 1 view .LVU70
	ldp	x21, x22, [sp, 32]
.LVL28:
	.loc 1 88 1 view .LVU71
	ldp	x29, x30, [sp], 48
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	.loc 1 86 3 view .LVU72
	br	x16
.LVL29:
	.p2align 2,,3
.L8:
	.cfi_restore_state
	.loc 1 88 1 view .LVU73
	ldp	x19, x20, [sp, 16]
.LVL30:
	.loc 1 88 1 view .LVU74
	ldp	x21, x22, [sp, 32]
.LVL31:
	.loc 1 88 1 view .LVU75
	ldp	x29, x30, [sp], 48
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL32:
.L12:
	.cfi_restore_state
	.loc 1 80 3 is_stmt 1 view .LVU76
	.loc 1 80 40 is_stmt 0 view .LVU77
	bl	__errno_location
.LVL33:
	.loc 1 80 3 view .LVU78
	ldr	w0, [x0]
	bl	strerror
.LVL34:
	mov	x1, x0
	adrp	x0, .LC1
	add	x0, x0, :lo12:.LC1
	bl	fatalf
.LVL35:
	.cfi_endproc
.LFE54:
	.size	x_cgo_init, .-x_cgo_init
	.comm	x_cgo_inittls,8,8
	.bss
	.align	3
	.set	.LANCHOR0,. + 0
	.type	setg_gcc, %object
	.size	setg_gcc, 8
setg_gcc:
	.zero	8
	.text
.Letext0:
	.file 2 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 3 "/usr/include/aarch64-linux-gnu/bits/pthreadtypes.h"
	.file 4 "/usr/include/aarch64-linux-gnu/bits/types/__sigset_t.h"
	.file 5 "/usr/include/aarch64-linux-gnu/bits/types/sigset_t.h"
	.file 6 "/usr/include/stdint.h"
	.file 7 "libcgo.h"
	.file 8 "libcgo_unix.h"
	.file 9 "/usr/include/stdlib.h"
	.file 10 "/usr/include/string.h"
	.file 11 "/usr/include/pthread.h"
	.file 12 "/usr/include/aarch64-linux-gnu/bits/sigthread.h"
	.file 13 "/usr/include/signal.h"
	.file 14 "/usr/include/errno.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x6f2
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x1a
	.4byte	.LASF49
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x5
	.byte	0x1
	.byte	0x8
	.4byte	.LASF2
	.uleb128 0x5
	.byte	0x2
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x5
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x5
	.byte	0x8
	.byte	0x7
	.4byte	.LASF5
	.uleb128 0x5
	.byte	0x1
	.byte	0x6
	.4byte	.LASF6
	.uleb128 0x5
	.byte	0x2
	.byte	0x5
	.4byte	.LASF7
	.uleb128 0x1b
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x5
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x1c
	.byte	0x8
	.uleb128 0x3
	.4byte	0x6d
	.uleb128 0x5
	.byte	0x1
	.byte	0x8
	.4byte	.LASF9
	.uleb128 0xa
	.4byte	0x6d
	.uleb128 0x6
	.4byte	.LASF11
	.byte	0x2
	.byte	0xd1
	.byte	0x17
	.4byte	0x43
	.uleb128 0x3
	.4byte	0x74
	.uleb128 0x5
	.byte	0x8
	.byte	0x7
	.4byte	.LASF10
	.uleb128 0x6
	.4byte	.LASF12
	.byte	0x3
	.byte	0x1b
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x1d
	.4byte	.LASF15
	.byte	0x40
	.byte	0x3
	.byte	0x38
	.byte	0x7
	.4byte	0xc1
	.uleb128 0x11
	.4byte	.LASF13
	.byte	0x3a
	.byte	0x8
	.4byte	0xc1
	.uleb128 0x11
	.4byte	.LASF14
	.byte	0x3b
	.byte	0xc
	.4byte	0x5f
	.byte	0
	.uleb128 0x12
	.4byte	0x6d
	.4byte	0xd1
	.uleb128 0x13
	.4byte	0x43
	.byte	0x3f
	.byte	0
	.uleb128 0x6
	.4byte	.LASF15
	.byte	0x3
	.byte	0x3e
	.byte	0x1e
	.4byte	0x9d
	.uleb128 0xa
	.4byte	0xd1
	.uleb128 0x5
	.byte	0x8
	.byte	0x5
	.4byte	.LASF16
	.uleb128 0x1e
	.byte	0x80
	.byte	0x4
	.byte	0x5
	.byte	0x9
	.4byte	0x100
	.uleb128 0xb
	.4byte	.LASF24
	.byte	0x4
	.byte	0x7
	.byte	0x15
	.4byte	0x100
	.byte	0
	.byte	0
	.uleb128 0x12
	.4byte	0x43
	.4byte	0x110
	.uleb128 0x13
	.4byte	0x43
	.byte	0xf
	.byte	0
	.uleb128 0x6
	.4byte	.LASF17
	.byte	0x4
	.byte	0x8
	.byte	0x3
	.4byte	0xe9
	.uleb128 0xa
	.4byte	0x110
	.uleb128 0x1f
	.byte	0x7
	.byte	0x4
	.4byte	0x3c
	.byte	0xb
	.byte	0x26
	.byte	0x1
	.4byte	0x13c
	.uleb128 0x14
	.4byte	.LASF18
	.byte	0
	.uleb128 0x14
	.4byte	.LASF19
	.byte	0x1
	.byte	0
	.uleb128 0x15
	.4byte	0x147
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x3
	.4byte	0x13c
	.uleb128 0x6
	.4byte	.LASF20
	.byte	0x5
	.byte	0x7
	.byte	0x14
	.4byte	0x110
	.uleb128 0x3
	.4byte	0xd1
	.uleb128 0x20
	.uleb128 0x3
	.4byte	0x15d
	.uleb128 0x5
	.byte	0x10
	.byte	0x7
	.4byte	.LASF21
	.uleb128 0x6
	.4byte	.LASF22
	.byte	0x6
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x6
	.4byte	.LASF23
	.byte	0x7
	.byte	0xf
	.byte	0x13
	.4byte	0x16a
	.uleb128 0x21
	.string	"G"
	.byte	0x7
	.byte	0x16
	.byte	0x12
	.4byte	0x18c
	.uleb128 0x22
	.string	"G"
	.byte	0x10
	.byte	0x7
	.byte	0x17
	.byte	0x8
	.4byte	0x1b2
	.uleb128 0xb
	.4byte	.LASF25
	.byte	0x7
	.byte	0x19
	.byte	0xa
	.4byte	0x176
	.byte	0
	.uleb128 0xb
	.4byte	.LASF26
	.byte	0x7
	.byte	0x1a
	.byte	0xa
	.4byte	0x176
	.byte	0x8
	.byte	0
	.uleb128 0x6
	.4byte	.LASF27
	.byte	0x7
	.byte	0x21
	.byte	0x1c
	.4byte	0x1be
	.uleb128 0x23
	.4byte	.LASF27
	.byte	0x18
	.byte	0x7
	.byte	0x22
	.byte	0x8
	.4byte	0x1ed
	.uleb128 0xc
	.string	"g"
	.byte	0x24
	.byte	0x5
	.4byte	0x1ed
	.byte	0
	.uleb128 0xc
	.string	"tls"
	.byte	0x25
	.byte	0xb
	.4byte	0x1f2
	.byte	0x8
	.uleb128 0xc
	.string	"fn"
	.byte	0x26
	.byte	0x9
	.4byte	0x15e
	.byte	0x10
	.byte	0
	.uleb128 0x3
	.4byte	0x182
	.uleb128 0x3
	.4byte	0x176
	.uleb128 0x3
	.4byte	0x1b2
	.uleb128 0x3
	.4byte	0x201
	.uleb128 0x24
	.4byte	0x66
	.4byte	0x210
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x15
	.4byte	0x220
	.uleb128 0x2
	.4byte	0x220
	.uleb128 0x2
	.4byte	0x220
	.byte	0
	.uleb128 0x3
	.4byte	0x66
	.uleb128 0x25
	.4byte	.LASF43
	.byte	0x1
	.byte	0xf
	.byte	0x8
	.4byte	0x23b
	.uleb128 0x9
	.byte	0x3
	.8byte	x_cgo_inittls
	.uleb128 0x3
	.4byte	0x210
	.uleb128 0x8
	.4byte	.LASF45
	.byte	0x10
	.byte	0xf
	.4byte	0x147
	.uleb128 0x9
	.byte	0x3
	.8byte	setg_gcc
	.uleb128 0x16
	.4byte	.LASF28
	.byte	0x8
	.byte	0x8
	.4byte	0x26b
	.uleb128 0x2
	.4byte	0x1ed
	.uleb128 0x2
	.4byte	0x1f2
	.byte	0
	.uleb128 0x26
	.4byte	.LASF50
	.byte	0xe
	.byte	0x25
	.byte	0xd
	.4byte	0x277
	.uleb128 0x3
	.4byte	0x58
	.uleb128 0x7
	.4byte	.LASF31
	.byte	0x9
	.2byte	0x21c
	.byte	0xe
	.4byte	0x66
	.4byte	0x293
	.uleb128 0x2
	.4byte	0x79
	.byte	0
	.uleb128 0x16
	.4byte	.LASF29
	.byte	0x1
	.byte	0x2c
	.4byte	0x2ae
	.uleb128 0x2
	.4byte	0x15e
	.uleb128 0x2
	.4byte	0x147
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x27
	.4byte	.LASF30
	.byte	0x9
	.2byte	0x22b
	.byte	0xd
	.4byte	0x2c1
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x28
	.4byte	.LASF32
	.byte	0x7
	.byte	0x4f
	.byte	0x6
	.4byte	0x2d4
	.uleb128 0x2
	.4byte	0x85
	.uleb128 0x29
	.byte	0
	.uleb128 0x7
	.4byte	.LASF33
	.byte	0xa
	.2byte	0x1a3
	.byte	0xe
	.4byte	0x68
	.4byte	0x2eb
	.uleb128 0x2
	.4byte	0x58
	.byte	0
	.uleb128 0xd
	.4byte	.LASF34
	.byte	0x8
	.byte	0xd
	.4byte	0x58
	.4byte	0x30f
	.uleb128 0x2
	.4byte	0x30f
	.uleb128 0x2
	.4byte	0x314
	.uleb128 0x2
	.4byte	0x1fc
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x3
	.4byte	0x91
	.uleb128 0x3
	.4byte	0xdd
	.uleb128 0x9
	.4byte	0x314
	.uleb128 0x7
	.4byte	.LASF35
	.byte	0xb
	.2byte	0x16e
	.byte	0xc
	.4byte	0x58
	.4byte	0x33a
	.uleb128 0x2
	.4byte	0x319
	.uleb128 0x2
	.4byte	0x33f
	.byte	0
	.uleb128 0x3
	.4byte	0x79
	.uleb128 0x9
	.4byte	0x33a
	.uleb128 0x7
	.4byte	.LASF36
	.byte	0xb
	.2byte	0x129
	.byte	0xc
	.4byte	0x58
	.4byte	0x360
	.uleb128 0x2
	.4byte	0x158
	.uleb128 0x2
	.4byte	0x58
	.byte	0
	.uleb128 0x7
	.4byte	.LASF37
	.byte	0xb
	.2byte	0x11d
	.byte	0xc
	.4byte	0x58
	.4byte	0x377
	.uleb128 0x2
	.4byte	0x158
	.byte	0
	.uleb128 0xd
	.4byte	.LASF38
	.byte	0xc
	.byte	0x1f
	.4byte	0x58
	.4byte	0x396
	.uleb128 0x2
	.4byte	0x58
	.uleb128 0x2
	.4byte	0x39b
	.uleb128 0x2
	.4byte	0x3a5
	.byte	0
	.uleb128 0x3
	.4byte	0x11c
	.uleb128 0x9
	.4byte	0x396
	.uleb128 0x3
	.4byte	0x110
	.uleb128 0x9
	.4byte	0x3a0
	.uleb128 0xd
	.4byte	.LASF39
	.byte	0xd
	.byte	0xca
	.4byte	0x58
	.4byte	0x3bf
	.uleb128 0x2
	.4byte	0x3bf
	.byte	0
	.uleb128 0x3
	.4byte	0x14c
	.uleb128 0x2a
	.4byte	.LASF51
	.byte	0x1
	.byte	0x3a
	.byte	0x1
	.8byte	.LFB54
	.8byte	.LFE54-.LFB54
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x4dc
	.uleb128 0xe
	.string	"g"
	.byte	0x3a
	.byte	0xf
	.4byte	0x1ed
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0xf
	.4byte	.LASF40
	.byte	0x19
	.4byte	0x147
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0xf
	.4byte	.LASF41
	.byte	0x2e
	.4byte	0x220
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0xf
	.4byte	.LASF42
	.byte	0x3b
	.4byte	0x220
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x2b
	.4byte	.LASF44
	.byte	0x1
	.byte	0x3c
	.byte	0xb
	.4byte	0x1f2
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x4
	.8byte	.LVL23
	.4byte	0x27c
	.4byte	0x454
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x40
	.byte	0
	.uleb128 0x4
	.8byte	.LVL25
	.4byte	0x255
	.4byte	0x472
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x4
	.8byte	.LVL26
	.4byte	0x2ae
	.4byte	0x48a
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x2c
	.8byte	.LVL29
	.4byte	0x4a6
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0
	.uleb128 0x17
	.8byte	.LVL33
	.4byte	0x26b
	.uleb128 0x17
	.8byte	.LVL34
	.4byte	0x2d4
	.uleb128 0x10
	.8byte	.LVL35
	.4byte	0x2c1
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC1
	.byte	0
	.byte	0
	.uleb128 0x2d
	.4byte	.LASF52
	.byte	0x1
	.byte	0x2e
	.byte	0x1
	.4byte	0x66
	.8byte	.LFB53
	.8byte	.LFE53-.LFB53
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x555
	.uleb128 0xe
	.string	"v"
	.byte	0x2e
	.byte	0x13
	.4byte	0x66
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x18
	.string	"ts"
	.byte	0x30
	.byte	0xe
	.4byte	0x1b2
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x4
	.8byte	.LVL3
	.4byte	0x2ae
	.4byte	0x53a
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0
	.uleb128 0x10
	.8byte	.LVL4
	.4byte	0x293
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.byte	0
	.uleb128 0x2e
	.4byte	.LASF53
	.byte	0x1
	.byte	0x13
	.byte	0x1
	.8byte	.LFB52
	.8byte	.LFE52-.LFB52
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0xe
	.string	"ts"
	.byte	0x13
	.byte	0x24
	.4byte	0x1f7
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x8
	.4byte	.LASF46
	.byte	0x15
	.byte	0x11
	.4byte	0xd1
	.uleb128 0x3
	.byte	0x91
	.sleb128 -320
	.uleb128 0x19
	.string	"ign"
	.byte	0x16
	.byte	0xb
	.4byte	0x14c
	.uleb128 0x3
	.byte	0x91
	.sleb128 -256
	.uleb128 0x8
	.4byte	.LASF47
	.byte	0x16
	.byte	0x10
	.4byte	0x14c
	.uleb128 0x3
	.byte	0x91
	.sleb128 -128
	.uleb128 0x19
	.string	"p"
	.byte	0x17
	.byte	0xc
	.4byte	0x91
	.uleb128 0x3
	.byte	0x91
	.sleb128 -336
	.uleb128 0x8
	.4byte	.LASF48
	.byte	0x18
	.byte	0x9
	.4byte	0x79
	.uleb128 0x3
	.byte	0x91
	.sleb128 -328
	.uleb128 0x18
	.string	"err"
	.byte	0x19
	.byte	0x6
	.4byte	0x58
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x4
	.8byte	.LVL7
	.4byte	0x3aa
	.4byte	0x5f5
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x4
	.8byte	.LVL8
	.4byte	0x377
	.4byte	0x619
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x32
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0x91
	.sleb128 -256
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.uleb128 0x4
	.8byte	.LVL9
	.4byte	0x360
	.4byte	0x631
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x4
	.8byte	.LVL10
	.4byte	0x344
	.4byte	0x64e
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.byte	0
	.uleb128 0x4
	.8byte	.LVL11
	.4byte	0x31e
	.4byte	0x66d
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0x91
	.sleb128 -328
	.byte	0
	.uleb128 0x4
	.8byte	.LVL12
	.4byte	0x2eb
	.4byte	0x69f
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0x91
	.sleb128 -336
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x9
	.byte	0x3
	.8byte	threadentry
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x2
	.byte	0x85
	.sleb128 0
	.byte	0
	.uleb128 0x4
	.8byte	.LVL15
	.4byte	0x377
	.4byte	0x6c1
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x32
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.byte	0x30
	.byte	0
	.uleb128 0x4
	.8byte	.LVL19
	.4byte	0x2d4
	.4byte	0x6d9
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x10
	.8byte	.LVL20
	.4byte	0x2c1
	.uleb128 0x1
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC0
	.byte	0
	.byte	0
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x2
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x3
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x4
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x5
	.uleb128 0x24
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3e
	.uleb128 0xb
	.uleb128 0x3
	.uleb128 0xe
	.byte	0
	.byte	0
	.uleb128 0x6
	.uleb128 0x16
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x7
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0x5
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x8
	.uleb128 0x34
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x9
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xa
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xb
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xc
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 7
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xd
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 12
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xe
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0xf
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 58
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x11
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 3
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x12
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x13
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x14
	.uleb128 0x28
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x1c
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x15
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x16
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 13
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x17
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x18
	.uleb128 0x34
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x19
	.uleb128 0x34
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x1a
	.uleb128 0x11
	.byte	0x1
	.uleb128 0x25
	.uleb128 0xe
	.uleb128 0x13
	.uleb128 0xb
	.uleb128 0x3
	.uleb128 0x1f
	.uleb128 0x1b
	.uleb128 0x1f
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x10
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x1b
	.uleb128 0x24
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3e
	.uleb128 0xb
	.uleb128 0x3
	.uleb128 0x8
	.byte	0
	.byte	0
	.uleb128 0x1c
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x1d
	.uleb128 0x17
	.byte	0x1
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1e
	.uleb128 0x13
	.byte	0x1
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1f
	.uleb128 0x4
	.byte	0x1
	.uleb128 0x3e
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x20
	.uleb128 0x15
	.byte	0
	.uleb128 0x27
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x21
	.uleb128 0x16
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x22
	.uleb128 0x13
	.byte	0x1
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x23
	.uleb128 0x13
	.byte	0x1
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x24
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x25
	.uleb128 0x34
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x26
	.uleb128 0x2e
	.byte	0
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x27
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0x5
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x28
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x87
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x29
	.uleb128 0x18
	.byte	0
	.byte	0
	.byte	0
	.uleb128 0x2a
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x40
	.uleb128 0x18
	.uleb128 0x7a
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2b
	.uleb128 0x34
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x2c
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x82
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2d
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x40
	.uleb128 0x18
	.uleb128 0x7a
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2e
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x40
	.uleb128 0x18
	.uleb128 0x7a
	.uleb128 0x19
	.byte	0
	.byte	0
	.byte	0
	.section	.debug_loclists,"",@progbits
	.4byte	.Ldebug_loc3-.Ldebug_loc2
.Ldebug_loc2:
	.2byte	0x5
	.byte	0x8
	.byte	0
	.4byte	0
.Ldebug_loc0:
.LVUS4:
	.uleb128 0
	.uleb128 .LVU56
	.uleb128 .LVU56
	.uleb128 .LVU69
	.uleb128 .LVU69
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU74
	.uleb128 .LVU74
	.uleb128 .LVU76
	.uleb128 .LVU76
	.uleb128 0
.LLST4:
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LVL27-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL27-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LVL30-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL30-.Ltext0
	.uleb128 .LVL32-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL32-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0
.LVUS5:
	.uleb128 0
	.uleb128 .LVU59
	.uleb128 .LVU59
	.uleb128 0
.LLST5:
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS6:
	.uleb128 0
	.uleb128 .LVU59
	.uleb128 .LVU59
	.uleb128 .LVU71
	.uleb128 .LVU71
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU75
	.uleb128 .LVU75
	.uleb128 .LVU76
	.uleb128 .LVU76
	.uleb128 0
.LLST6:
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 0x1
	.byte	0x52
	.byte	0x4
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 .LVL28-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL28-.Ltext0
	.uleb128 .LVL29-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL29-1-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LVL31-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL31-.Ltext0
	.uleb128 .LVL32-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL32-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS7:
	.uleb128 0
	.uleb128 .LVU59
	.uleb128 .LVU59
	.uleb128 .LVU71
	.uleb128 .LVU71
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU75
	.uleb128 .LVU75
	.uleb128 .LVU76
	.uleb128 .LVU76
	.uleb128 0
.LLST7:
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 0x1
	.byte	0x53
	.byte	0x4
	.uleb128 .LVL23-1-.Ltext0
	.uleb128 .LVL28-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0x4
	.uleb128 .LVL28-.Ltext0
	.uleb128 .LVL29-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL29-1-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LVL31-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0x4
	.uleb128 .LVL31-.Ltext0
	.uleb128 .LVL32-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL32-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0
.LVUS8:
	.uleb128 .LVU59
	.uleb128 .LVU62
	.uleb128 .LVU62
	.uleb128 .LVU63
	.uleb128 .LVU63
	.uleb128 .LVU69
	.uleb128 .LVU73
	.uleb128 .LVU74
	.uleb128 .LVU76
	.uleb128 .LVU78
.LLST8:
	.byte	0x4
	.uleb128 .LVL23-.Ltext0
	.uleb128 .LVL24-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL24-.Ltext0
	.uleb128 .LVL25-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL25-1-.Ltext0
	.uleb128 .LVL27-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LVL30-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL32-.Ltext0
	.uleb128 .LVL33-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS0:
	.uleb128 0
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 .LFE53-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU5
	.uleb128 .LVU6
	.uleb128 .LVU6
	.uleb128 .LVU9
.LLST1:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x5
	.byte	0x64
	.byte	0x93
	.uleb128 0x8
	.byte	0x93
	.uleb128 0x10
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x8
	.byte	0x64
	.byte	0x93
	.uleb128 0x8
	.byte	0x93
	.uleb128 0x8
	.byte	0x63
	.byte	0x93
	.uleb128 0x8
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU23
	.uleb128 .LVU23
	.uleb128 .LVU44
	.uleb128 .LVU44
	.uleb128 .LVU45
	.uleb128 .LVU45
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL17-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL17-.Ltext0
	.uleb128 .LVL18-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LFE52-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS3:
	.uleb128 .LVU38
	.uleb128 .LVU39
	.uleb128 .LVU39
	.uleb128 .LVU43
	.uleb128 .LVU45
	.uleb128 0
.LLST3:
	.byte	0x4
	.uleb128 .LVL13-.Ltext0
	.uleb128 .LVL14-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL14-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LFE52-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.Ldebug_loc3:
	.section	.debug_aranges,"",@progbits
	.4byte	0x2c
	.2byte	0x2
	.4byte	.Ldebug_info0
	.byte	0x8
	.byte	0
	.2byte	0
	.2byte	0
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.8byte	0
	.8byte	0
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF26:
	.string	"stackhi"
.LASF32:
	.string	"fatalf"
.LASF21:
	.string	"__int128 unsigned"
.LASF47:
	.string	"oset"
.LASF22:
	.string	"uintptr_t"
.LASF38:
	.string	"pthread_sigmask"
.LASF8:
	.string	"long int"
.LASF24:
	.string	"__val"
.LASF19:
	.string	"PTHREAD_CREATE_DETACHED"
.LASF18:
	.string	"PTHREAD_CREATE_JOINABLE"
.LASF28:
	.string	"_cgo_set_stacklo"
.LASF12:
	.string	"pthread_t"
.LASF48:
	.string	"size"
.LASF13:
	.string	"__size"
.LASF37:
	.string	"pthread_attr_init"
.LASF45:
	.string	"setg_gcc"
.LASF20:
	.string	"sigset_t"
.LASF3:
	.string	"short unsigned int"
.LASF11:
	.string	"size_t"
.LASF46:
	.string	"attr"
.LASF36:
	.string	"pthread_attr_setdetachstate"
.LASF2:
	.string	"unsigned char"
.LASF53:
	.string	"_cgo_sys_thread_start"
.LASF42:
	.string	"tlsbase"
.LASF35:
	.string	"pthread_attr_getstacksize"
.LASF43:
	.string	"x_cgo_inittls"
.LASF4:
	.string	"unsigned int"
.LASF34:
	.string	"_cgo_try_pthread_create"
.LASF52:
	.string	"threadentry"
.LASF10:
	.string	"long long unsigned int"
.LASF25:
	.string	"stacklo"
.LASF6:
	.string	"signed char"
.LASF5:
	.string	"long unsigned int"
.LASF27:
	.string	"ThreadStart"
.LASF44:
	.string	"pbounds"
.LASF30:
	.string	"free"
.LASF16:
	.string	"long long int"
.LASF9:
	.string	"char"
.LASF49:
	.string	"GNU C17 11.4.0"
.LASF14:
	.string	"__align"
.LASF40:
	.string	"setg"
.LASF7:
	.string	"short int"
.LASF17:
	.string	"__sigset_t"
.LASF33:
	.string	"strerror"
.LASF41:
	.string	"tlsg"
.LASF15:
	.string	"pthread_attr_t"
.LASF29:
	.string	"crosscall1"
.LASF50:
	.string	"__errno_location"
.LASF23:
	.string	"uintptr"
.LASF51:
	.string	"x_cgo_init"
.LASF31:
	.string	"malloc"
.LASF39:
	.string	"sigfillset"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_linux_arm64.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
