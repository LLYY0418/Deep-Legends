	.arch armv8-a
	.file	"gcc_util.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_util.c"
	.section	.rodata.str1.8,"aMS",@progbits,1
	.align	3
.LC0:
	.string	"runtime/cgo: out of memory in thread_start\n"
	.text
	.align	2
	.p2align 4,,11
	.global	x_cgo_thread_start
	.type	x_cgo_thread_start, %function
x_cgo_thread_start:
.LVL0:
.LFB39:
	.file 1 "gcc_util.c"
	.loc 1 10 1 view -0
	.cfi_startproc
	.loc 1 11 2 view .LVU1
	.loc 1 14 21 view .LVU2
	.loc 1 15 2 view .LVU3
	.loc 1 10 1 is_stmt 0 view .LVU4
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 10 1 view .LVU5
	mov	x19, x0
	.loc 1 15 7 view .LVU6
	mov	x0, 24
.LVL1:
	.loc 1 15 7 view .LVU7
	bl	malloc
.LVL2:
	.loc 1 16 21 is_stmt 1 view .LVU8
	.loc 1 17 2 view .LVU9
	.loc 1 17 4 is_stmt 0 view .LVU10
	cbz	x0, .L5
	.loc 1 21 2 is_stmt 1 view .LVU11
	.loc 1 21 6 is_stmt 0 view .LVU12
	ldp	x2, x3, [x19]
	stp	x2, x3, [x0]
	ldr	x2, [x19, 16]
	.loc 1 24 1 view .LVU13
	ldr	x19, [sp, 16]
.LVL3:
	.loc 1 21 6 view .LVU14
	str	x2, [x0, 16]
	.loc 1 23 2 is_stmt 1 view .LVU15
	.loc 1 24 1 is_stmt 0 view .LVU16
	ldp	x29, x30, [sp], 32
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	.loc 1 23 2 view .LVU17
	b	_cgo_sys_thread_start
.LVL4:
.L5:
	.cfi_restore_state
	.loc 1 18 3 is_stmt 1 view .LVU18
.LBB4:
.LBI4:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/stdio2.h"
	.loc 2 103 1 view .LVU19
.LBB5:
	.loc 2 105 3 view .LVU20
.LBE5:
.LBE4:
	.loc 1 18 3 is_stmt 0 view .LVU21
	adrp	x3, :got:stderr
.LBB8:
.LBB6:
	.loc 2 105 10 view .LVU22
	mov	x2, 43
	mov	x1, 1
	adrp	x0, .LC0
.LVL5:
	.loc 2 105 10 view .LVU23
.LBE6:
.LBE8:
	.loc 1 18 3 view .LVU24
	ldr	x3, [x3, #:got_lo12:stderr]
.LBB9:
.LBB7:
	.loc 2 105 10 view .LVU25
	add	x0, x0, :lo12:.LC0
	ldr	x3, [x3]
	bl	fwrite
.LVL6:
	.loc 2 105 10 view .LVU26
.LBE7:
.LBE9:
	.loc 1 19 3 is_stmt 1 view .LVU27
	bl	abort
.LVL7:
	.cfi_endproc
.LFE39:
	.size	x_cgo_thread_start, .-x_cgo_thread_start
	.global	_cgo_yield
	.section	.rodata
	.align	3
	.type	_cgo_yield, %object
	.size	_cgo_yield, 8
_cgo_yield:
	.zero	8
	.text
.Letext0:
	.file 3 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 4 "/usr/include/stdint.h"
	.file 5 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 6 "/usr/include/aarch64-linux-gnu/bits/types/struct_FILE.h"
	.file 7 "/usr/include/aarch64-linux-gnu/bits/types/FILE.h"
	.file 8 "libcgo.h"
	.file 9 "/usr/include/stdio.h"
	.file 10 "/usr/include/stdlib.h"
	.file 11 "<built-in>"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x4b8
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x10
	.4byte	.LASF60
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x3
	.byte	0x1
	.byte	0x8
	.4byte	.LASF2
	.uleb128 0x3
	.byte	0x2
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x3
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x3
	.byte	0x8
	.byte	0x7
	.4byte	.LASF5
	.uleb128 0x3
	.byte	0x1
	.byte	0x6
	.4byte	.LASF6
	.uleb128 0x3
	.byte	0x2
	.byte	0x5
	.4byte	.LASF7
	.uleb128 0x11
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x3
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x4
	.4byte	.LASF9
	.byte	0x3
	.byte	0x98
	.byte	0x19
	.4byte	0x5f
	.uleb128 0x4
	.4byte	.LASF10
	.byte	0x3
	.byte	0x99
	.byte	0x1b
	.4byte	0x5f
	.uleb128 0x12
	.byte	0x8
	.uleb128 0x2
	.4byte	0x85
	.uleb128 0x3
	.byte	0x1
	.byte	0x8
	.4byte	.LASF11
	.uleb128 0xa
	.4byte	0x85
	.uleb128 0x4
	.4byte	.LASF12
	.byte	0x4
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x4
	.4byte	.LASF13
	.byte	0x5
	.byte	0xd1
	.byte	0x17
	.4byte	0x43
	.uleb128 0x3
	.byte	0x8
	.byte	0x5
	.4byte	.LASF14
	.uleb128 0x3
	.byte	0x8
	.byte	0x7
	.4byte	.LASF15
	.uleb128 0xb
	.4byte	.LASF50
	.byte	0xd8
	.byte	0x6
	.byte	0x31
	.4byte	0x23d
	.uleb128 0x1
	.4byte	.LASF16
	.byte	0x6
	.byte	0x33
	.byte	0x7
	.4byte	0x58
	.byte	0
	.uleb128 0x1
	.4byte	.LASF17
	.byte	0x6
	.byte	0x36
	.byte	0x9
	.4byte	0x80
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF18
	.byte	0x6
	.byte	0x37
	.byte	0x9
	.4byte	0x80
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF19
	.byte	0x6
	.byte	0x38
	.byte	0x9
	.4byte	0x80
	.byte	0x18
	.uleb128 0x1
	.4byte	.LASF20
	.byte	0x6
	.byte	0x39
	.byte	0x9
	.4byte	0x80
	.byte	0x20
	.uleb128 0x1
	.4byte	.LASF21
	.byte	0x6
	.byte	0x3a
	.byte	0x9
	.4byte	0x80
	.byte	0x28
	.uleb128 0x1
	.4byte	.LASF22
	.byte	0x6
	.byte	0x3b
	.byte	0x9
	.4byte	0x80
	.byte	0x30
	.uleb128 0x1
	.4byte	.LASF23
	.byte	0x6
	.byte	0x3c
	.byte	0x9
	.4byte	0x80
	.byte	0x38
	.uleb128 0x1
	.4byte	.LASF24
	.byte	0x6
	.byte	0x3d
	.byte	0x9
	.4byte	0x80
	.byte	0x40
	.uleb128 0x1
	.4byte	.LASF25
	.byte	0x6
	.byte	0x40
	.byte	0x9
	.4byte	0x80
	.byte	0x48
	.uleb128 0x1
	.4byte	.LASF26
	.byte	0x6
	.byte	0x41
	.byte	0x9
	.4byte	0x80
	.byte	0x50
	.uleb128 0x1
	.4byte	.LASF27
	.byte	0x6
	.byte	0x42
	.byte	0x9
	.4byte	0x80
	.byte	0x58
	.uleb128 0x1
	.4byte	.LASF28
	.byte	0x6
	.byte	0x44
	.byte	0x16
	.4byte	0x256
	.byte	0x60
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0x6
	.byte	0x46
	.byte	0x14
	.4byte	0x25b
	.byte	0x68
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0x6
	.byte	0x48
	.byte	0x7
	.4byte	0x58
	.byte	0x70
	.uleb128 0x1
	.4byte	.LASF31
	.byte	0x6
	.byte	0x49
	.byte	0x7
	.4byte	0x58
	.byte	0x74
	.uleb128 0x1
	.4byte	.LASF32
	.byte	0x6
	.byte	0x4a
	.byte	0xb
	.4byte	0x66
	.byte	0x78
	.uleb128 0x1
	.4byte	.LASF33
	.byte	0x6
	.byte	0x4d
	.byte	0x12
	.4byte	0x35
	.byte	0x80
	.uleb128 0x1
	.4byte	.LASF34
	.byte	0x6
	.byte	0x4e
	.byte	0xf
	.4byte	0x4a
	.byte	0x82
	.uleb128 0x1
	.4byte	.LASF35
	.byte	0x6
	.byte	0x4f
	.byte	0x8
	.4byte	0x260
	.byte	0x83
	.uleb128 0x1
	.4byte	.LASF36
	.byte	0x6
	.byte	0x51
	.byte	0xf
	.4byte	0x270
	.byte	0x88
	.uleb128 0x1
	.4byte	.LASF37
	.byte	0x6
	.byte	0x59
	.byte	0xd
	.4byte	0x72
	.byte	0x90
	.uleb128 0x1
	.4byte	.LASF38
	.byte	0x6
	.byte	0x5b
	.byte	0x17
	.4byte	0x27a
	.byte	0x98
	.uleb128 0x1
	.4byte	.LASF39
	.byte	0x6
	.byte	0x5c
	.byte	0x19
	.4byte	0x284
	.byte	0xa0
	.uleb128 0x1
	.4byte	.LASF40
	.byte	0x6
	.byte	0x5d
	.byte	0x14
	.4byte	0x25b
	.byte	0xa8
	.uleb128 0x1
	.4byte	.LASF41
	.byte	0x6
	.byte	0x5e
	.byte	0x9
	.4byte	0x7e
	.byte	0xb0
	.uleb128 0x1
	.4byte	.LASF42
	.byte	0x6
	.byte	0x5f
	.byte	0xa
	.4byte	0x9d
	.byte	0xb8
	.uleb128 0x1
	.4byte	.LASF43
	.byte	0x6
	.byte	0x60
	.byte	0x7
	.4byte	0x58
	.byte	0xc0
	.uleb128 0x1
	.4byte	.LASF44
	.byte	0x6
	.byte	0x62
	.byte	0x8
	.4byte	0x289
	.byte	0xc4
	.byte	0
	.uleb128 0x4
	.4byte	.LASF45
	.byte	0x7
	.byte	0x7
	.byte	0x19
	.4byte	0xb7
	.uleb128 0x13
	.4byte	.LASF61
	.byte	0x6
	.byte	0x2b
	.byte	0xe
	.uleb128 0x7
	.4byte	.LASF46
	.uleb128 0x2
	.4byte	0x251
	.uleb128 0x2
	.4byte	0xb7
	.uleb128 0xc
	.4byte	0x85
	.4byte	0x270
	.uleb128 0xd
	.4byte	0x43
	.byte	0
	.byte	0
	.uleb128 0x2
	.4byte	0x249
	.uleb128 0x7
	.4byte	.LASF47
	.uleb128 0x2
	.4byte	0x275
	.uleb128 0x7
	.4byte	.LASF48
	.uleb128 0x2
	.4byte	0x27f
	.uleb128 0xc
	.4byte	0x85
	.4byte	0x299
	.uleb128 0xd
	.4byte	0x43
	.byte	0x13
	.byte	0
	.uleb128 0x2
	.4byte	0x23d
	.uleb128 0xe
	.4byte	0x299
	.uleb128 0x14
	.4byte	.LASF54
	.byte	0x9
	.byte	0x91
	.byte	0xe
	.4byte	0x299
	.uleb128 0x4
	.4byte	.LASF49
	.byte	0x8
	.byte	0xf
	.byte	0x13
	.4byte	0x91
	.uleb128 0x15
	.string	"G"
	.byte	0x8
	.byte	0x16
	.byte	0x12
	.4byte	0x2c5
	.uleb128 0x16
	.string	"G"
	.byte	0x10
	.byte	0x8
	.byte	0x17
	.byte	0x8
	.4byte	0x2eb
	.uleb128 0x1
	.4byte	.LASF51
	.byte	0x8
	.byte	0x19
	.byte	0xa
	.4byte	0x2af
	.byte	0
	.uleb128 0x1
	.4byte	.LASF52
	.byte	0x8
	.byte	0x1a
	.byte	0xa
	.4byte	0x2af
	.byte	0x8
	.byte	0
	.uleb128 0x4
	.4byte	.LASF53
	.byte	0x8
	.byte	0x21
	.byte	0x1c
	.4byte	0x2f7
	.uleb128 0xb
	.4byte	.LASF53
	.byte	0x18
	.byte	0x8
	.byte	0x22
	.4byte	0x325
	.uleb128 0x8
	.string	"g"
	.byte	0x24
	.byte	0x5
	.4byte	0x325
	.byte	0
	.uleb128 0x8
	.string	"tls"
	.byte	0x25
	.byte	0xb
	.4byte	0x32a
	.byte	0x8
	.uleb128 0x8
	.string	"fn"
	.byte	0x26
	.byte	0x9
	.4byte	0x330
	.byte	0x10
	.byte	0
	.uleb128 0x2
	.4byte	0x2bb
	.uleb128 0x2
	.4byte	0x2af
	.uleb128 0x17
	.uleb128 0x2
	.4byte	0x32f
	.uleb128 0x2
	.4byte	0x2eb
	.uleb128 0x18
	.4byte	0x341
	.uleb128 0x9
	.byte	0
	.uleb128 0x19
	.4byte	.LASF55
	.byte	0x1
	.byte	0x1b
	.byte	0xe
	.4byte	0x35c
	.uleb128 0x9
	.byte	0x3
	.8byte	_cgo_yield
	.uleb128 0x2
	.4byte	0x33a
	.uleb128 0xa
	.4byte	0x357
	.uleb128 0x1a
	.4byte	.LASF56
	.byte	0x2
	.byte	0x5d
	.byte	0xc
	.4byte	0x58
	.4byte	0x382
	.uleb128 0x5
	.4byte	0x29e
	.uleb128 0x5
	.4byte	0x58
	.uleb128 0x5
	.4byte	0x387
	.uleb128 0x9
	.byte	0
	.uleb128 0x2
	.4byte	0x8c
	.uleb128 0xe
	.4byte	0x382
	.uleb128 0x1b
	.4byte	.LASF62
	.byte	0x8
	.byte	0x3e
	.byte	0x6
	.4byte	0x39e
	.uleb128 0x5
	.4byte	0x335
	.byte	0
	.uleb128 0x1c
	.4byte	.LASF63
	.byte	0xa
	.2byte	0x256
	.byte	0xd
	.uleb128 0x1d
	.4byte	.LASF57
	.byte	0xa
	.2byte	0x21c
	.byte	0xe
	.4byte	0x7e
	.4byte	0x3be
	.uleb128 0x5
	.4byte	0x9d
	.byte	0
	.uleb128 0x1e
	.4byte	.LASF64
	.byte	0x1
	.byte	0x9
	.byte	0x1
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x487
	.uleb128 0x1f
	.string	"arg"
	.byte	0x1
	.byte	0x9
	.byte	0x21
	.4byte	0x335
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x20
	.string	"ts"
	.byte	0x1
	.byte	0xb
	.byte	0xf
	.4byte	0x335
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x21
	.4byte	0x487
	.8byte	.LBI4
	.byte	.LVU19
	.4byte	.LLRL2
	.byte	0x1
	.byte	0x12
	.byte	0x3
	.4byte	0x455
	.uleb128 0x22
	.4byte	0x4a3
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x23
	.4byte	0x498
	.uleb128 0x24
	.8byte	.LVL6
	.4byte	0x4b0
	.uleb128 0x6
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC0
	.uleb128 0x6
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.uleb128 0x6
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x8
	.byte	0x2b
	.byte	0
	.byte	0
	.uleb128 0x25
	.8byte	.LVL2
	.4byte	0x3a7
	.4byte	0x46c
	.uleb128 0x6
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x48
	.byte	0
	.uleb128 0x26
	.8byte	.LVL4
	.4byte	0x38c
	.uleb128 0x27
	.8byte	.LVL7
	.4byte	0x39e
	.byte	0
	.uleb128 0x28
	.4byte	.LASF65
	.byte	0x2
	.byte	0x67
	.byte	0x1
	.4byte	0x58
	.byte	0x3
	.4byte	0x4b0
	.uleb128 0xf
	.4byte	.LASF58
	.byte	0x67
	.byte	0x1b
	.4byte	0x29e
	.uleb128 0xf
	.4byte	.LASF59
	.byte	0x67
	.byte	0x3c
	.4byte	0x387
	.uleb128 0x9
	.byte	0
	.uleb128 0x29
	.4byte	.LASF66
	.4byte	.LASF67
	.byte	0xb
	.byte	0
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
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
	.uleb128 0x2
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x3
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
	.uleb128 0x4
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
	.uleb128 0x5
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x6
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x7
	.uleb128 0x13
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x8
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 8
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
	.uleb128 0x9
	.uleb128 0x18
	.byte	0
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
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xc
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xd
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xe
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xf
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
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
	.uleb128 0x11
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
	.uleb128 0x12
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x13
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
	.byte	0
	.byte	0
	.uleb128 0x14
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
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x15
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
	.uleb128 0x16
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
	.uleb128 0x17
	.uleb128 0x15
	.byte	0
	.uleb128 0x27
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x18
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x19
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
	.uleb128 0x1a
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
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1b
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
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1c
	.uleb128 0x2e
	.byte	0
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
	.uleb128 0x87
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x1d
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
	.uleb128 0x1e
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
	.uleb128 0x1f
	.uleb128 0x5
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
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x20
	.uleb128 0x34
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
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x21
	.uleb128 0x1d
	.byte	0x1
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x52
	.uleb128 0x1
	.uleb128 0x2138
	.uleb128 0xb
	.uleb128 0x55
	.uleb128 0x17
	.uleb128 0x58
	.uleb128 0xb
	.uleb128 0x59
	.uleb128 0xb
	.uleb128 0x57
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x22
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x23
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x24
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x25
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
	.uleb128 0x26
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x82
	.uleb128 0x19
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x27
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
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
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x20
	.uleb128 0xb
	.uleb128 0x34
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x29
	.uleb128 0x2e
	.byte	0
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x6e
	.uleb128 0xe
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
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
.LVUS0:
	.uleb128 0
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU14
	.uleb128 .LVU14
	.uleb128 .LVU18
	.uleb128 .LVU18
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS1:
	.uleb128 .LVU8
	.uleb128 .LVU18
	.uleb128 .LVU18
	.uleb128 .LVU23
.LLST1:
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS3:
	.uleb128 .LVU19
	.uleb128 .LVU26
.LLST3:
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0xa
	.byte	0x3
	.8byte	.LC0
	.byte	0x9f
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
	.section	.debug_rnglists,"",@progbits
.Ldebug_ranges0:
	.4byte	.Ldebug_ranges3-.Ldebug_ranges2
.Ldebug_ranges2:
	.2byte	0x5
	.byte	0x8
	.byte	0
	.4byte	0
.LLRL2:
	.byte	0x4
	.uleb128 .LBB4-.Ltext0
	.uleb128 .LBE4-.Ltext0
	.byte	0x4
	.uleb128 .LBB8-.Ltext0
	.uleb128 .LBE8-.Ltext0
	.byte	0x4
	.uleb128 .LBB9-.Ltext0
	.uleb128 .LBE9-.Ltext0
	.byte	0
.Ldebug_ranges3:
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF9:
	.string	"__off_t"
.LASF17:
	.string	"_IO_read_ptr"
.LASF57:
	.string	"malloc"
.LASF29:
	.string	"_chain"
.LASF13:
	.string	"size_t"
.LASF12:
	.string	"uintptr_t"
.LASF35:
	.string	"_shortbuf"
.LASF60:
	.string	"GNU C17 11.4.0"
.LASF23:
	.string	"_IO_buf_base"
.LASF15:
	.string	"long long unsigned int"
.LASF38:
	.string	"_codecvt"
.LASF14:
	.string	"long long int"
.LASF6:
	.string	"signed char"
.LASF67:
	.string	"__builtin_fwrite"
.LASF30:
	.string	"_fileno"
.LASF18:
	.string	"_IO_read_end"
.LASF8:
	.string	"long int"
.LASF16:
	.string	"_flags"
.LASF24:
	.string	"_IO_buf_end"
.LASF33:
	.string	"_cur_column"
.LASF55:
	.string	"_cgo_yield"
.LASF47:
	.string	"_IO_codecvt"
.LASF53:
	.string	"ThreadStart"
.LASF32:
	.string	"_old_offset"
.LASF37:
	.string	"_offset"
.LASF51:
	.string	"stacklo"
.LASF46:
	.string	"_IO_marker"
.LASF4:
	.string	"unsigned int"
.LASF41:
	.string	"_freeres_buf"
.LASF65:
	.string	"fprintf"
.LASF58:
	.string	"__stream"
.LASF5:
	.string	"long unsigned int"
.LASF21:
	.string	"_IO_write_ptr"
.LASF3:
	.string	"short unsigned int"
.LASF25:
	.string	"_IO_save_base"
.LASF36:
	.string	"_lock"
.LASF31:
	.string	"_flags2"
.LASF43:
	.string	"_mode"
.LASF22:
	.string	"_IO_write_end"
.LASF49:
	.string	"uintptr"
.LASF61:
	.string	"_IO_lock_t"
.LASF50:
	.string	"_IO_FILE"
.LASF52:
	.string	"stackhi"
.LASF28:
	.string	"_markers"
.LASF2:
	.string	"unsigned char"
.LASF7:
	.string	"short int"
.LASF64:
	.string	"x_cgo_thread_start"
.LASF48:
	.string	"_IO_wide_data"
.LASF34:
	.string	"_vtable_offset"
.LASF45:
	.string	"FILE"
.LASF56:
	.string	"__fprintf_chk"
.LASF62:
	.string	"_cgo_sys_thread_start"
.LASF11:
	.string	"char"
.LASF63:
	.string	"abort"
.LASF10:
	.string	"__off64_t"
.LASF19:
	.string	"_IO_read_base"
.LASF27:
	.string	"_IO_save_end"
.LASF59:
	.string	"__fmt"
.LASF42:
	.string	"__pad5"
.LASF44:
	.string	"_unused2"
.LASF54:
	.string	"stderr"
.LASF26:
	.string	"_IO_backup_base"
.LASF66:
	.string	"fwrite"
.LASF40:
	.string	"_freeres_list"
.LASF39:
	.string	"_wide_data"
.LASF20:
	.string	"_IO_write_base"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_util.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
