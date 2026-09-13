	.arch armv8-a
	.file	"gcc_fatalf.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_fatalf.c"
	.section	.rodata.str1.8,"aMS",@progbits,1
	.align	3
.LC0:
	.string	"runtime/cgo: "
	.text
	.align	2
	.p2align 4,,11
	.global	fatalf
	.type	fatalf, %function
fatalf:
.LVL0:
.LFB39:
	.file 1 "gcc_fatalf.c"
	.loc 1 14 1 view -0
	.cfi_startproc
	.loc 1 15 2 view .LVU1
	.loc 1 17 2 view .LVU2
.LBB8:
.LBI8:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/stdio2.h"
	.loc 2 103 1 view .LVU3
.LBB9:
	.loc 2 105 3 view .LVU4
.LBE9:
.LBE8:
	.loc 1 14 1 is_stmt 0 view .LVU5
	stp	x29, x30, [sp, -320]!
	.cfi_def_cfa_offset 320
	.cfi_offset 29, -320
	.cfi_offset 30, -312
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -304
	.cfi_offset 20, -296
	.loc 1 17 2 view .LVU6
	adrp	x19, :got:stderr
	.loc 1 14 1 view .LVU7
	mov	x20, x0
	.loc 1 17 2 view .LVU8
	ldr	x19, [x19, #:got_lo12:stderr]
	.loc 1 14 1 view .LVU9
	str	q0, [sp, 128]
	str	x3, [sp, 280]
.LBB13:
.LBB10:
	.loc 2 105 10 view .LVU10
	adrp	x3, .LC0
	add	x0, x3, :lo12:.LC0
.LVL1:
	.loc 2 105 10 view .LVU11
	ldr	x3, [x19]
.LBE10:
.LBE13:
	.loc 1 14 1 view .LVU12
	str	q1, [sp, 144]
	str	q2, [sp, 160]
	str	q3, [sp, 176]
	str	q4, [sp, 192]
	str	q5, [sp, 208]
	str	q6, [sp, 224]
	str	q7, [sp, 240]
	stp	x1, x2, [sp, 264]
.LBB14:
.LBB11:
	.loc 2 105 10 view .LVU13
	mov	x1, 1
	mov	x2, 13
.LBE11:
.LBE14:
	.loc 1 14 1 view .LVU14
	stp	x4, x5, [sp, 288]
	stp	x6, x7, [sp, 304]
.LBB15:
.LBB12:
	.loc 2 105 10 view .LVU15
	bl	fwrite
.LVL2:
	.loc 2 105 10 view .LVU16
.LBE12:
.LBE15:
	.loc 1 18 2 is_stmt 1 view .LVU17
	add	x2, sp, 256
	add	x3, sp, 320
	mov	w1, -56
	mov	w0, -128
	stp	x3, x3, [sp, 64]
.LBB16:
.LBB17:
	.loc 2 135 10 is_stmt 0 view .LVU18
	add	x3, sp, 32
.LBE17:
.LBE16:
	.loc 1 18 2 view .LVU19
	str	x2, [sp, 80]
.LBB21:
.LBB18:
	.loc 2 135 10 view .LVU20
	mov	x2, x20
.LBE18:
.LBE21:
	.loc 1 18 2 view .LVU21
	stp	w1, w0, [sp, 88]
	.loc 1 19 2 is_stmt 1 view .LVU22
.LBB22:
.LBB19:
	.loc 2 135 10 is_stmt 0 view .LVU23
	mov	w1, 1
.LBE19:
.LBE22:
	.loc 1 19 2 view .LVU24
	ldr	x0, [x19]
.LVL3:
.LBB23:
.LBI16:
	.loc 2 132 1 is_stmt 1 view .LVU25
.LBB20:
	.loc 2 135 3 view .LVU26
	ldp	q0, q1, [sp, 64]
	stp	q0, q1, [sp, 96]
	.loc 2 135 10 is_stmt 0 view .LVU27
	stp	q0, q1, [x3]
	bl	__vfprintf_chk
.LVL4:
	.loc 2 135 10 view .LVU28
.LBE20:
.LBE23:
	.loc 1 20 2 is_stmt 1 view .LVU29
	.loc 1 21 2 view .LVU30
.LBB24:
.LBI24:
	.loc 2 103 1 view .LVU31
.LBB25:
	.loc 2 105 3 view .LVU32
	.loc 2 105 10 is_stmt 0 view .LVU33
	mov	w0, 10
	ldr	x1, [x19]
	bl	fputc
.LVL5:
	.loc 2 105 10 view .LVU34
.LBE25:
.LBE24:
	.loc 1 22 2 is_stmt 1 view .LVU35
	bl	abort
.LVL6:
	.cfi_endproc
.LFE39:
	.size	fatalf, .-fatalf
.Letext0:
	.file 3 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stdarg.h"
	.file 4 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 5 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 6 "/usr/include/aarch64-linux-gnu/bits/types/struct_FILE.h"
	.file 7 "/usr/include/aarch64-linux-gnu/bits/types/FILE.h"
	.file 8 "<built-in>"
	.file 9 "/usr/include/stdio.h"
	.file 10 "/usr/include/stdlib.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x4e3
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x15
	.4byte	.LASF66
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x6
	.4byte	.LASF7
	.byte	0x3
	.byte	0x28
	.byte	0x1b
	.4byte	0x3a
	.uleb128 0x16
	.4byte	.LASF67
	.byte	0x20
	.byte	0x8
	.byte	0
	.4byte	0x79
	.uleb128 0x7
	.4byte	.LASF2
	.4byte	0x79
	.byte	0
	.uleb128 0x7
	.4byte	.LASF3
	.4byte	0x79
	.byte	0x8
	.uleb128 0x7
	.4byte	.LASF4
	.4byte	0x79
	.byte	0x10
	.uleb128 0x7
	.4byte	.LASF5
	.4byte	0x7b
	.byte	0x18
	.uleb128 0x7
	.4byte	.LASF6
	.4byte	0x7b
	.byte	0x1c
	.byte	0
	.uleb128 0x17
	.byte	0x8
	.uleb128 0x18
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x6
	.4byte	.LASF8
	.byte	0x3
	.byte	0x63
	.byte	0x18
	.4byte	0x2e
	.uleb128 0x6
	.4byte	.LASF9
	.byte	0x4
	.byte	0xd1
	.byte	0x17
	.4byte	0x9a
	.uleb128 0x2
	.byte	0x8
	.byte	0x7
	.4byte	.LASF10
	.uleb128 0x2
	.byte	0x1
	.byte	0x8
	.4byte	.LASF11
	.uleb128 0x2
	.byte	0x2
	.byte	0x7
	.4byte	.LASF12
	.uleb128 0x2
	.byte	0x4
	.byte	0x7
	.4byte	.LASF13
	.uleb128 0x2
	.byte	0x1
	.byte	0x6
	.4byte	.LASF14
	.uleb128 0x2
	.byte	0x2
	.byte	0x5
	.4byte	.LASF15
	.uleb128 0x2
	.byte	0x8
	.byte	0x5
	.4byte	.LASF16
	.uleb128 0x6
	.4byte	.LASF17
	.byte	0x5
	.byte	0x98
	.byte	0x19
	.4byte	0xc4
	.uleb128 0x6
	.4byte	.LASF18
	.byte	0x5
	.byte	0x99
	.byte	0x1b
	.4byte	0xc4
	.uleb128 0x3
	.4byte	0xe8
	.uleb128 0x2
	.byte	0x1
	.byte	0x8
	.4byte	.LASF19
	.uleb128 0x19
	.4byte	0xe8
	.uleb128 0x1a
	.4byte	.LASF68
	.byte	0xd8
	.byte	0x6
	.byte	0x31
	.byte	0x8
	.4byte	0x25e
	.uleb128 0x1
	.4byte	.LASF20
	.byte	0x33
	.byte	0x7
	.4byte	0x7b
	.byte	0
	.uleb128 0x1
	.4byte	.LASF21
	.byte	0x36
	.byte	0x9
	.4byte	0xe3
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF22
	.byte	0x37
	.byte	0x9
	.4byte	0xe3
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF23
	.byte	0x38
	.byte	0x9
	.4byte	0xe3
	.byte	0x18
	.uleb128 0x1
	.4byte	.LASF24
	.byte	0x39
	.byte	0x9
	.4byte	0xe3
	.byte	0x20
	.uleb128 0x1
	.4byte	.LASF25
	.byte	0x3a
	.byte	0x9
	.4byte	0xe3
	.byte	0x28
	.uleb128 0x1
	.4byte	.LASF26
	.byte	0x3b
	.byte	0x9
	.4byte	0xe3
	.byte	0x30
	.uleb128 0x1
	.4byte	.LASF27
	.byte	0x3c
	.byte	0x9
	.4byte	0xe3
	.byte	0x38
	.uleb128 0x1
	.4byte	.LASF28
	.byte	0x3d
	.byte	0x9
	.4byte	0xe3
	.byte	0x40
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0x40
	.byte	0x9
	.4byte	0xe3
	.byte	0x48
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0x41
	.byte	0x9
	.4byte	0xe3
	.byte	0x50
	.uleb128 0x1
	.4byte	.LASF31
	.byte	0x42
	.byte	0x9
	.4byte	0xe3
	.byte	0x58
	.uleb128 0x1
	.4byte	.LASF32
	.byte	0x44
	.byte	0x16
	.4byte	0x277
	.byte	0x60
	.uleb128 0x1
	.4byte	.LASF33
	.byte	0x46
	.byte	0x14
	.4byte	0x27c
	.byte	0x68
	.uleb128 0x1
	.4byte	.LASF34
	.byte	0x48
	.byte	0x7
	.4byte	0x7b
	.byte	0x70
	.uleb128 0x1
	.4byte	.LASF35
	.byte	0x49
	.byte	0x7
	.4byte	0x7b
	.byte	0x74
	.uleb128 0x1
	.4byte	.LASF36
	.byte	0x4a
	.byte	0xb
	.4byte	0xcb
	.byte	0x78
	.uleb128 0x1
	.4byte	.LASF37
	.byte	0x4d
	.byte	0x12
	.4byte	0xa8
	.byte	0x80
	.uleb128 0x1
	.4byte	.LASF38
	.byte	0x4e
	.byte	0xf
	.4byte	0xb6
	.byte	0x82
	.uleb128 0x1
	.4byte	.LASF39
	.byte	0x4f
	.byte	0x8
	.4byte	0x281
	.byte	0x83
	.uleb128 0x1
	.4byte	.LASF40
	.byte	0x51
	.byte	0xf
	.4byte	0x291
	.byte	0x88
	.uleb128 0x1
	.4byte	.LASF41
	.byte	0x59
	.byte	0xd
	.4byte	0xd7
	.byte	0x90
	.uleb128 0x1
	.4byte	.LASF42
	.byte	0x5b
	.byte	0x17
	.4byte	0x29b
	.byte	0x98
	.uleb128 0x1
	.4byte	.LASF43
	.byte	0x5c
	.byte	0x19
	.4byte	0x2a5
	.byte	0xa0
	.uleb128 0x1
	.4byte	.LASF44
	.byte	0x5d
	.byte	0x14
	.4byte	0x27c
	.byte	0xa8
	.uleb128 0x1
	.4byte	.LASF45
	.byte	0x5e
	.byte	0x9
	.4byte	0x79
	.byte	0xb0
	.uleb128 0x1
	.4byte	.LASF46
	.byte	0x5f
	.byte	0xa
	.4byte	0x8e
	.byte	0xb8
	.uleb128 0x1
	.4byte	.LASF47
	.byte	0x60
	.byte	0x7
	.4byte	0x7b
	.byte	0xc0
	.uleb128 0x1
	.4byte	.LASF48
	.byte	0x62
	.byte	0x8
	.4byte	0x2aa
	.byte	0xc4
	.byte	0
	.uleb128 0x6
	.4byte	.LASF49
	.byte	0x7
	.byte	0x7
	.byte	0x19
	.4byte	0xf4
	.uleb128 0x1b
	.4byte	.LASF69
	.byte	0x6
	.byte	0x2b
	.byte	0xe
	.uleb128 0xa
	.4byte	.LASF50
	.uleb128 0x3
	.4byte	0x272
	.uleb128 0x3
	.4byte	0xf4
	.uleb128 0xd
	.4byte	0xe8
	.4byte	0x291
	.uleb128 0xe
	.4byte	0x9a
	.byte	0
	.byte	0
	.uleb128 0x3
	.4byte	0x26a
	.uleb128 0xa
	.4byte	.LASF51
	.uleb128 0x3
	.4byte	0x296
	.uleb128 0xa
	.4byte	.LASF52
	.uleb128 0x3
	.4byte	0x2a0
	.uleb128 0xd
	.4byte	0xe8
	.4byte	0x2ba
	.uleb128 0xe
	.4byte	0x9a
	.byte	0x13
	.byte	0
	.uleb128 0x3
	.4byte	0x25e
	.uleb128 0xf
	.4byte	0x2ba
	.uleb128 0x1c
	.4byte	.LASF70
	.byte	0x9
	.byte	0x91
	.byte	0xe
	.4byte	0x2ba
	.uleb128 0x2
	.byte	0x8
	.byte	0x5
	.4byte	.LASF53
	.uleb128 0x2
	.byte	0x8
	.byte	0x7
	.4byte	.LASF54
	.uleb128 0x10
	.4byte	.LASF55
	.byte	0x5d
	.4byte	0x7b
	.4byte	0x2fd
	.uleb128 0x4
	.4byte	0x2bf
	.uleb128 0x4
	.4byte	0x7b
	.uleb128 0x4
	.4byte	0x302
	.uleb128 0xb
	.byte	0
	.uleb128 0x3
	.4byte	0xef
	.uleb128 0xf
	.4byte	0x2fd
	.uleb128 0x10
	.4byte	.LASF56
	.byte	0x60
	.4byte	0x7b
	.4byte	0x32a
	.uleb128 0x4
	.4byte	0x2bf
	.uleb128 0x4
	.4byte	0x7b
	.uleb128 0x4
	.4byte	0x302
	.uleb128 0x4
	.4byte	0x2e
	.byte	0
	.uleb128 0x1d
	.4byte	.LASF71
	.byte	0xa
	.2byte	0x256
	.byte	0xd
	.uleb128 0x1e
	.4byte	.LASF72
	.byte	0x1
	.byte	0xd
	.byte	0x1
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x478
	.uleb128 0x1f
	.4byte	.LASF73
	.byte	0x1
	.byte	0xd
	.byte	0x14
	.4byte	0x2fd
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0xb
	.uleb128 0x20
	.string	"ap"
	.byte	0x1
	.byte	0xf
	.byte	0xa
	.4byte	0x82
	.uleb128 0x3
	.byte	0x91
	.sleb128 -256
	.uleb128 0x11
	.4byte	0x4a8
	.8byte	.LBI8
	.byte	.LVU3
	.4byte	.LLRL1
	.byte	0x11
	.4byte	0x3c4
	.uleb128 0x9
	.4byte	0x4c1
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x12
	.4byte	0x4b6
	.uleb128 0xc
	.8byte	.LVL2
	.4byte	0x4ce
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC0
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.byte	0x3d
	.byte	0
	.byte	0
	.uleb128 0x11
	.4byte	0x478
	.8byte	.LBI16
	.byte	.LVU25
	.4byte	.LLRL3
	.byte	0x13
	.4byte	0x41f
	.uleb128 0x21
	.4byte	0x49c
	.uleb128 0x3
	.byte	0x91
	.sleb128 -224
	.uleb128 0x9
	.4byte	0x491
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x9
	.4byte	0x486
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0xc
	.8byte	.LVL4
	.4byte	0x307
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x3
	.byte	0x91
	.sleb128 -288
	.byte	0
	.byte	0
	.uleb128 0x22
	.4byte	0x4a8
	.8byte	.LBI24
	.byte	.LVU31
	.8byte	.LBB24
	.8byte	.LBE24-.LBB24
	.byte	0x1
	.byte	0x15
	.byte	0x2
	.4byte	0x46a
	.uleb128 0x9
	.4byte	0x4c1
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x12
	.4byte	0x4b6
	.uleb128 0xc
	.8byte	.LVL5
	.4byte	0x4dd
	.uleb128 0x5
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x3a
	.byte	0
	.byte	0
	.uleb128 0x23
	.8byte	.LVL6
	.4byte	0x32a
	.byte	0
	.uleb128 0x13
	.4byte	.LASF60
	.byte	0x84
	.4byte	0x7b
	.4byte	0x4a8
	.uleb128 0x8
	.4byte	.LASF57
	.byte	0x84
	.byte	0x1c
	.4byte	0x2bf
	.uleb128 0x8
	.4byte	.LASF58
	.byte	0x85
	.byte	0x1b
	.4byte	0x302
	.uleb128 0x8
	.4byte	.LASF59
	.byte	0x85
	.byte	0x31
	.4byte	0x2e
	.byte	0
	.uleb128 0x13
	.4byte	.LASF61
	.byte	0x67
	.4byte	0x7b
	.4byte	0x4ce
	.uleb128 0x8
	.4byte	.LASF57
	.byte	0x67
	.byte	0x1b
	.4byte	0x2bf
	.uleb128 0x8
	.4byte	.LASF58
	.byte	0x67
	.byte	0x3c
	.4byte	0x302
	.uleb128 0xb
	.byte	0
	.uleb128 0x14
	.4byte	.LASF62
	.4byte	.LASF64
	.uleb128 0x24
	.uleb128 0x4
	.byte	0x9e
	.uleb128 0x2
	.byte	0xa
	.byte	0
	.uleb128 0x14
	.4byte	.LASF63
	.4byte	.LASF65
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 6
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
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x5
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
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
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.uleb128 0x34
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x8
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
	.uleb128 0x9
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
	.uleb128 0xa
	.uleb128 0x13
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0xb
	.uleb128 0x18
	.byte	0
	.byte	0
	.byte	0
	.uleb128 0xc
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xd
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xe
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xf
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
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
	.uleb128 0x11
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
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x59
	.uleb128 0xb
	.uleb128 0x57
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x12
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x13
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x20
	.uleb128 0x21
	.sleb128 3
	.uleb128 0x34
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x14
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
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 0
	.byte	0
	.byte	0
	.uleb128 0x15
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
	.uleb128 0x16
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
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x17
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x18
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
	.uleb128 0x19
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1a
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
	.uleb128 0x1b
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
	.uleb128 0x1c
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
	.uleb128 0x1d
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
	.uleb128 0x87
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
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x21
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x22
	.uleb128 0x1d
	.byte	0x1
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x52
	.uleb128 0x1
	.uleb128 0x2138
	.uleb128 0xb
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
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
	.uleb128 0x23
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x24
	.uleb128 0x36
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
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
	.uleb128 .LVU11
	.uleb128 .LVU11
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0
.LVUS2:
	.uleb128 .LVU3
	.uleb128 .LVU16
.LLST2:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0xa
	.byte	0x3
	.8byte	.LC0
	.byte	0x9f
	.byte	0
.LVUS4:
	.uleb128 .LVU25
	.uleb128 .LVU28
.LLST4:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0
.LVUS5:
	.uleb128 .LVU25
	.uleb128 .LVU28
.LLST5:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS6:
	.uleb128 .LVU31
	.uleb128 .LVU34
.LLST6:
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x6
	.byte	0xa0
	.4byte	.Ldebug_info0+1239
	.sleb128 0
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
.LLRL1:
	.byte	0x4
	.uleb128 .LBB8-.Ltext0
	.uleb128 .LBE8-.Ltext0
	.byte	0x4
	.uleb128 .LBB13-.Ltext0
	.uleb128 .LBE13-.Ltext0
	.byte	0x4
	.uleb128 .LBB14-.Ltext0
	.uleb128 .LBE14-.Ltext0
	.byte	0x4
	.uleb128 .LBB15-.Ltext0
	.uleb128 .LBE15-.Ltext0
	.byte	0
.LLRL3:
	.byte	0x4
	.uleb128 .LBB16-.Ltext0
	.uleb128 .LBE16-.Ltext0
	.byte	0x4
	.uleb128 .LBB21-.Ltext0
	.uleb128 .LBE21-.Ltext0
	.byte	0x4
	.uleb128 .LBB22-.Ltext0
	.uleb128 .LBE22-.Ltext0
	.byte	0x4
	.uleb128 .LBB23-.Ltext0
	.uleb128 .LBE23-.Ltext0
	.byte	0
.Ldebug_ranges3:
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF17:
	.string	"__off_t"
.LASF21:
	.string	"_IO_read_ptr"
.LASF33:
	.string	"_chain"
.LASF9:
	.string	"size_t"
.LASF39:
	.string	"_shortbuf"
.LASF66:
	.string	"GNU C17 11.4.0"
.LASF27:
	.string	"_IO_buf_base"
.LASF54:
	.string	"long long unsigned int"
.LASF42:
	.string	"_codecvt"
.LASF53:
	.string	"long long int"
.LASF14:
	.string	"signed char"
.LASF64:
	.string	"__builtin_fwrite"
.LASF72:
	.string	"fatalf"
.LASF34:
	.string	"_fileno"
.LASF65:
	.string	"__builtin_fputc"
.LASF22:
	.string	"_IO_read_end"
.LASF16:
	.string	"long int"
.LASF20:
	.string	"_flags"
.LASF67:
	.string	"__va_list"
.LASF28:
	.string	"_IO_buf_end"
.LASF37:
	.string	"_cur_column"
.LASF51:
	.string	"_IO_codecvt"
.LASF36:
	.string	"_old_offset"
.LASF41:
	.string	"_offset"
.LASF5:
	.string	"__gr_offs"
.LASF8:
	.string	"va_list"
.LASF6:
	.string	"__vr_offs"
.LASF50:
	.string	"_IO_marker"
.LASF13:
	.string	"unsigned int"
.LASF45:
	.string	"_freeres_buf"
.LASF61:
	.string	"fprintf"
.LASF57:
	.string	"__stream"
.LASF10:
	.string	"long unsigned int"
.LASF25:
	.string	"_IO_write_ptr"
.LASF2:
	.string	"__stack"
.LASF29:
	.string	"_IO_save_base"
.LASF40:
	.string	"_lock"
.LASF35:
	.string	"_flags2"
.LASF47:
	.string	"_mode"
.LASF7:
	.string	"__gnuc_va_list"
.LASF26:
	.string	"_IO_write_end"
.LASF63:
	.string	"fputc"
.LASF69:
	.string	"_IO_lock_t"
.LASF68:
	.string	"_IO_FILE"
.LASF56:
	.string	"__vfprintf_chk"
.LASF32:
	.string	"_markers"
.LASF11:
	.string	"unsigned char"
.LASF3:
	.string	"__gr_top"
.LASF15:
	.string	"short int"
.LASF52:
	.string	"_IO_wide_data"
.LASF38:
	.string	"_vtable_offset"
.LASF49:
	.string	"FILE"
.LASF73:
	.string	"format"
.LASF55:
	.string	"__fprintf_chk"
.LASF19:
	.string	"char"
.LASF71:
	.string	"abort"
.LASF18:
	.string	"__off64_t"
.LASF23:
	.string	"_IO_read_base"
.LASF31:
	.string	"_IO_save_end"
.LASF59:
	.string	"__ap"
.LASF12:
	.string	"short unsigned int"
.LASF58:
	.string	"__fmt"
.LASF46:
	.string	"__pad5"
.LASF48:
	.string	"_unused2"
.LASF70:
	.string	"stderr"
.LASF62:
	.string	"fwrite"
.LASF30:
	.string	"_IO_backup_base"
.LASF60:
	.string	"vfprintf"
.LASF44:
	.string	"_freeres_list"
.LASF43:
	.string	"_wide_data"
.LASF24:
	.string	"_IO_write_base"
.LASF4:
	.string	"__vr_top"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_fatalf.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
