	.arch armv8-a
	.file	"cgo_resnew.cgo2.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT-build" "cgo_resnew.cgo2.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_C2func_getnameinfo
	.type	_cgo_77133bf98b3a_C2func_getnameinfo, %function
_cgo_77133bf98b3a_C2func_getnameinfo:
.LVL0:
.LFB23:
	.file 1 "cgo-gcc-prolog"
	.loc 1 46 1 view -0
	.cfi_startproc
	.loc 1 47 2 view .LVU1
	.loc 1 48 2 view .LVU2
	.loc 1 46 1 is_stmt 0 view .LVU3
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	mov	x19, x0
.LVL1:
	.loc 1 61 2 is_stmt 1 view .LVU4
	.loc 1 46 1 is_stmt 0 view .LVU5
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -16
	.cfi_offset 22, -8
	.loc 1 61 22 view .LVU6
	bl	_cgo_topofstack
.LVL2:
	.loc 1 61 22 view .LVU7
	mov	x22, x0
.LVL3:
	.loc 1 62 2 is_stmt 1 view .LVU8
	.loc 1 63 21 view .LVU9
	.loc 1 64 2 view .LVU10
	bl	__errno_location
.LVL4:
	.loc 1 64 2 is_stmt 0 view .LVU11
	mov	x20, x0
	.loc 1 65 11 view .LVU12
	ldr	x0, [x19]
	ldr	x2, [x19, 16]
	ldr	x4, [x19, 32]
	.loc 1 64 8 view .LVU13
	str	wzr, [x20]
	.loc 1 65 2 is_stmt 1 view .LVU14
	.loc 1 65 11 is_stmt 0 view .LVU15
	ldr	w1, [x19, 8]
	ldr	w3, [x19, 24]
	ldp	w5, w6, [x19, 40]
	bl	getnameinfo
.LVL5:
	.loc 1 66 13 view .LVU16
	ldr	w20, [x20]
	.loc 1 65 11 view .LVU17
	mov	w21, w0
.LVL6:
	.loc 1 66 2 is_stmt 1 view .LVU18
	.loc 1 67 21 view .LVU19
	.loc 1 68 2 view .LVU20
	.loc 1 68 36 is_stmt 0 view .LVU21
	bl	_cgo_topofstack
.LVL7:
	.loc 1 69 2 is_stmt 1 view .LVU22
	.loc 1 68 54 is_stmt 0 view .LVU23
	sub	x1, x0, x22
	.loc 1 72 1 view .LVU24
	mov	w0, w20
.LVL8:
	.loc 1 69 12 view .LVU25
	add	x19, x19, x1
.LVL9:
	.loc 1 69 12 view .LVU26
	str	w21, [x19, 48]
	.loc 1 70 48 is_stmt 1 view .LVU27
	.loc 1 71 2 view .LVU28
	.loc 1 72 1 is_stmt 0 view .LVU29
	ldp	x19, x20, [sp, 16]
.LVL10:
	.loc 1 72 1 view .LVU30
	ldp	x21, x22, [sp, 32]
.LVL11:
	.loc 1 72 1 view .LVU31
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE23:
	.size	_cgo_77133bf98b3a_C2func_getnameinfo, .-_cgo_77133bf98b3a_C2func_getnameinfo
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_getnameinfo
	.type	_cgo_77133bf98b3a_Cfunc_getnameinfo, %function
_cgo_77133bf98b3a_Cfunc_getnameinfo:
.LVL12:
.LFB24:
	.loc 1 77 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 78 2 view .LVU33
	.loc 1 77 1 is_stmt 0 view .LVU34
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	mov	x19, x0
.LVL13:
	.loc 1 91 2 is_stmt 1 view .LVU35
	.loc 1 77 1 is_stmt 0 view .LVU36
	str	x21, [sp, 32]
	.cfi_offset 21, -16
	.loc 1 91 22 view .LVU37
	bl	_cgo_topofstack
.LVL14:
	.loc 1 94 11 view .LVU38
	ldr	w1, [x19, 8]
	.loc 1 91 22 view .LVU39
	mov	x21, x0
.LVL15:
	.loc 1 92 2 is_stmt 1 view .LVU40
	.loc 1 93 21 view .LVU41
	.loc 1 94 2 view .LVU42
	.loc 1 94 11 is_stmt 0 view .LVU43
	ldr	w3, [x19, 24]
	ldp	w5, w6, [x19, 40]
	ldr	x0, [x19]
.LVL16:
	.loc 1 94 11 view .LVU44
	ldr	x2, [x19, 16]
	ldr	x4, [x19, 32]
	bl	getnameinfo
.LVL17:
	mov	w20, w0
.LVL18:
	.loc 1 95 21 is_stmt 1 view .LVU45
	.loc 1 96 2 view .LVU46
	.loc 1 96 36 is_stmt 0 view .LVU47
	bl	_cgo_topofstack
.LVL19:
	.loc 1 97 2 is_stmt 1 view .LVU48
	.loc 1 96 54 is_stmt 0 view .LVU49
	sub	x0, x0, x21
.LVL20:
	.loc 1 97 12 view .LVU50
	add	x19, x19, x0
.LVL21:
	.loc 1 99 1 view .LVU51
	ldr	x21, [sp, 32]
.LVL22:
	.loc 1 97 12 view .LVU52
	str	w20, [x19, 48]
	.loc 1 98 48 is_stmt 1 view .LVU53
	.loc 1 99 1 is_stmt 0 view .LVU54
	ldp	x19, x20, [sp, 16]
.LVL23:
	.loc 1 99 1 view .LVU55
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE24:
	.size	_cgo_77133bf98b3a_Cfunc_getnameinfo, .-_cgo_77133bf98b3a_Cfunc_getnameinfo
.Letext0:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 3 "/usr/include/aarch64-linux-gnu/bits/socket.h"
	.file 4 "/usr/include/aarch64-linux-gnu/bits/sockaddr.h"
	.file 5 "/usr/include/errno.h"
	.file 6 "/usr/include/netdb.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x3b0
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0xf
	.4byte	.LASF27
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x2
	.byte	0x8
	.byte	0x5
	.4byte	.LASF2
	.uleb128 0x2
	.byte	0x8
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x2
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x2
	.byte	0x8
	.byte	0x5
	.4byte	.LASF5
	.uleb128 0x2
	.byte	0x10
	.byte	0x4
	.4byte	.LASF6
	.uleb128 0x2
	.byte	0x1
	.byte	0x8
	.4byte	.LASF7
	.uleb128 0x7
	.4byte	0x51
	.uleb128 0x9
	.4byte	0x58
	.uleb128 0x2
	.byte	0x1
	.byte	0x8
	.4byte	.LASF8
	.uleb128 0x2
	.byte	0x2
	.byte	0x7
	.4byte	.LASF9
	.uleb128 0x2
	.byte	0x1
	.byte	0x6
	.4byte	.LASF10
	.uleb128 0x2
	.byte	0x2
	.byte	0x5
	.4byte	.LASF11
	.uleb128 0x10
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x11
	.byte	0x8
	.uleb128 0x8
	.4byte	.LASF13
	.byte	0x2
	.byte	0xd2
	.byte	0x17
	.4byte	0x3c
	.uleb128 0x2
	.byte	0x8
	.byte	0x7
	.4byte	.LASF12
	.uleb128 0x8
	.4byte	.LASF14
	.byte	0x3
	.byte	0x21
	.byte	0x15
	.4byte	0x87
	.uleb128 0x8
	.4byte	.LASF15
	.byte	0x4
	.byte	0x1c
	.byte	0x1c
	.4byte	0x69
	.uleb128 0x12
	.4byte	.LASF28
	.byte	0x10
	.byte	0x3
	.byte	0xb4
	.byte	0x8
	.4byte	0xda
	.uleb128 0x3
	.4byte	.LASF16
	.byte	0x3
	.byte	0xb6
	.byte	0x5
	.4byte	0xa6
	.byte	0
	.uleb128 0x3
	.4byte	.LASF17
	.byte	0x3
	.byte	0xb7
	.byte	0xa
	.4byte	0xdf
	.byte	0x2
	.byte	0
	.uleb128 0x13
	.4byte	0xb2
	.uleb128 0xa
	.4byte	0x51
	.4byte	0xef
	.uleb128 0xb
	.4byte	0x35
	.byte	0xd
	.byte	0
	.uleb128 0x14
	.4byte	.LASF29
	.byte	0x6
	.2byte	0x2a3
	.byte	0xc
	.4byte	0x7e
	.4byte	0x124
	.uleb128 0x4
	.4byte	0x129
	.uleb128 0x4
	.4byte	0x9a
	.uleb128 0x4
	.4byte	0x5d
	.uleb128 0x4
	.4byte	0x9a
	.uleb128 0x4
	.4byte	0x5d
	.uleb128 0x4
	.4byte	0x9a
	.uleb128 0x4
	.4byte	0x7e
	.byte	0
	.uleb128 0x7
	.4byte	0xda
	.uleb128 0x9
	.4byte	0x124
	.uleb128 0xc
	.4byte	.LASF18
	.byte	0x5
	.byte	0x25
	.byte	0xd
	.4byte	0x13a
	.uleb128 0x7
	.4byte	0x7e
	.uleb128 0xc
	.4byte	.LASF19
	.byte	0x1
	.byte	0x13
	.byte	0xe
	.4byte	0x58
	.uleb128 0x15
	.4byte	.LASF30
	.byte	0x1
	.byte	0x4c
	.byte	0x1
	.8byte	.LFB24
	.8byte	.LFE24-.LFB24
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x260
	.uleb128 0xd
	.string	"v"
	.byte	0x4c
	.byte	0x2b
	.4byte	0x85
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0xe
	.byte	0x4e
	.4byte	0x1ff
	.uleb128 0x1
	.string	"p0"
	.byte	0x4f
	.byte	0x1a
	.4byte	0x124
	.byte	0
	.uleb128 0x1
	.string	"p1"
	.byte	0x50
	.byte	0xd
	.4byte	0x9a
	.byte	0x8
	.uleb128 0x3
	.4byte	.LASF20
	.byte	0x1
	.byte	0x51
	.byte	0x8
	.4byte	0x260
	.byte	0xc
	.uleb128 0x1
	.string	"p2"
	.byte	0x52
	.byte	0x9
	.4byte	0x58
	.byte	0x10
	.uleb128 0x1
	.string	"p3"
	.byte	0x53
	.byte	0xd
	.4byte	0x9a
	.byte	0x18
	.uleb128 0x3
	.4byte	.LASF21
	.byte	0x1
	.byte	0x54
	.byte	0x8
	.4byte	0x260
	.byte	0x1c
	.uleb128 0x1
	.string	"p4"
	.byte	0x55
	.byte	0x9
	.4byte	0x58
	.byte	0x20
	.uleb128 0x1
	.string	"p5"
	.byte	0x56
	.byte	0xd
	.4byte	0x9a
	.byte	0x28
	.uleb128 0x1
	.string	"p6"
	.byte	0x57
	.byte	0x7
	.4byte	0x7e
	.byte	0x2c
	.uleb128 0x1
	.string	"r"
	.byte	0x58
	.byte	0x7
	.4byte	0x7e
	.byte	0x30
	.uleb128 0x3
	.4byte	.LASF22
	.byte	0x1
	.byte	0x59
	.byte	0x8
	.4byte	0x260
	.byte	0x34
	.byte	0
	.uleb128 0x5
	.4byte	.LASF23
	.byte	0x5a
	.byte	0x21
	.4byte	0x270
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x5
	.4byte	.LASF24
	.byte	0x5b
	.byte	0x8
	.4byte	0x58
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x5
	.4byte	.LASF25
	.byte	0x5c
	.byte	0x18
	.4byte	0x7e
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x6
	.8byte	.LVL14
	.4byte	0x13f
	.uleb128 0x6
	.8byte	.LVL17
	.4byte	0xef
	.uleb128 0x6
	.8byte	.LVL19
	.4byte	0x13f
	.byte	0
	.uleb128 0xa
	.4byte	0x51
	.4byte	0x270
	.uleb128 0xb
	.4byte	0x35
	.byte	0x3
	.byte	0
	.uleb128 0x7
	.4byte	0x17a
	.uleb128 0x16
	.4byte	.LASF31
	.byte	0x1
	.byte	0x2d
	.byte	0x1
	.4byte	0x7e
	.8byte	.LFB23
	.8byte	.LFE23-.LFB23
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x3ae
	.uleb128 0xd
	.string	"v"
	.byte	0x2d
	.byte	0x2c
	.4byte	0x85
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x5
	.4byte	.LASF26
	.byte	0x2f
	.byte	0x6
	.4byte	0x7e
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0xe
	.byte	0x30
	.4byte	0x340
	.uleb128 0x1
	.string	"p0"
	.byte	0x31
	.byte	0x1a
	.4byte	0x124
	.byte	0
	.uleb128 0x1
	.string	"p1"
	.byte	0x32
	.byte	0xd
	.4byte	0x9a
	.byte	0x8
	.uleb128 0x3
	.4byte	.LASF20
	.byte	0x1
	.byte	0x33
	.byte	0x8
	.4byte	0x260
	.byte	0xc
	.uleb128 0x1
	.string	"p2"
	.byte	0x34
	.byte	0x9
	.4byte	0x58
	.byte	0x10
	.uleb128 0x1
	.string	"p3"
	.byte	0x35
	.byte	0xd
	.4byte	0x9a
	.byte	0x18
	.uleb128 0x3
	.4byte	.LASF21
	.byte	0x1
	.byte	0x36
	.byte	0x8
	.4byte	0x260
	.byte	0x1c
	.uleb128 0x1
	.string	"p4"
	.byte	0x37
	.byte	0x9
	.4byte	0x58
	.byte	0x20
	.uleb128 0x1
	.string	"p5"
	.byte	0x38
	.byte	0xd
	.4byte	0x9a
	.byte	0x28
	.uleb128 0x1
	.string	"p6"
	.byte	0x39
	.byte	0x7
	.4byte	0x7e
	.byte	0x2c
	.uleb128 0x1
	.string	"r"
	.byte	0x3a
	.byte	0x7
	.4byte	0x7e
	.byte	0x30
	.uleb128 0x3
	.4byte	.LASF22
	.byte	0x1
	.byte	0x3b
	.byte	0x8
	.4byte	0x260
	.byte	0x34
	.byte	0
	.uleb128 0x5
	.4byte	.LASF23
	.byte	0x3c
	.byte	0x21
	.4byte	0x3ae
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x5
	.4byte	.LASF24
	.byte	0x3d
	.byte	0x8
	.4byte	0x58
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x5
	.4byte	.LASF25
	.byte	0x3e
	.byte	0x18
	.4byte	0x7e
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x6
	.8byte	.LVL2
	.4byte	0x13f
	.uleb128 0x6
	.8byte	.LVL4
	.4byte	0x12e
	.uleb128 0x6
	.8byte	.LVL5
	.4byte	0xef
	.uleb128 0x6
	.8byte	.LVL7
	.4byte	0x13f
	.byte	0
	.uleb128 0x7
	.4byte	0x2bb
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
	.uleb128 0xd
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
	.uleb128 0x4
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x5
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
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x6
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x7
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x8
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
	.uleb128 0x9
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xa
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xc
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
	.uleb128 0xd
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
	.uleb128 0xe
	.uleb128 0x13
	.byte	0x1
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 56
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xf
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
	.uleb128 0x10
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
	.uleb128 0x11
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x12
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
	.uleb128 0x13
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x14
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
	.uleb128 0x15
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
	.byte	0
	.section	.debug_loclists,"",@progbits
	.4byte	.Ldebug_loc3-.Ldebug_loc2
.Ldebug_loc2:
	.2byte	0x5
	.byte	0x8
	.byte	0
	.4byte	0
.Ldebug_loc0:
.LVUS5:
	.uleb128 0
	.uleb128 .LVU38
	.uleb128 .LVU38
	.uleb128 .LVU51
	.uleb128 .LVU51
	.uleb128 0
.LLST5:
	.byte	0x4
	.uleb128 .LVL12-.Ltext0
	.uleb128 .LVL14-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL14-1-.Ltext0
	.uleb128 .LVL21-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LFE24-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS6:
	.uleb128 .LVU35
	.uleb128 .LVU38
	.uleb128 .LVU38
	.uleb128 .LVU48
	.uleb128 .LVU48
	.uleb128 .LVU50
.LLST6:
	.byte	0x4
	.uleb128 .LVL13-.Ltext0
	.uleb128 .LVL14-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL14-1-.Ltext0
	.uleb128 .LVL19-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL19-.Ltext0
	.uleb128 .LVL20-.Ltext0
	.uleb128 0x9
	.byte	0x83
	.sleb128 0
	.byte	0x85
	.sleb128 0
	.byte	0x1c
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x9f
	.byte	0
.LVUS7:
	.uleb128 .LVU40
	.uleb128 .LVU44
	.uleb128 .LVU44
	.uleb128 .LVU52
.LLST7:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL16-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS8:
	.uleb128 .LVU45
	.uleb128 .LVU48
	.uleb128 .LVU48
	.uleb128 .LVU55
	.uleb128 .LVU55
	.uleb128 0
.LLST8:
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LVL19-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL19-1-.Ltext0
	.uleb128 .LVL23-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL23-.Ltext0
	.uleb128 .LFE24-.Ltext0
	.uleb128 0x8
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x23
	.uleb128 0x30
	.byte	0
.LVUS0:
	.uleb128 0
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU26
	.uleb128 .LVU26
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 .LVL9-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL9-.Ltext0
	.uleb128 .LFE23-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU19
	.uleb128 .LVU30
	.uleb128 .LVU30
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL10-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL10-.Ltext0
	.uleb128 .LFE23-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS2:
	.uleb128 .LVU4
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU22
	.uleb128 .LVU22
	.uleb128 .LVU25
.LLST2:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 .LVL7-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LVL8-.Ltext0
	.uleb128 0x9
	.byte	0x83
	.sleb128 0
	.byte	0x86
	.sleb128 0
	.byte	0x1c
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x9f
	.byte	0
.LVUS3:
	.uleb128 .LVU8
	.uleb128 .LVU11
	.uleb128 .LVU11
	.uleb128 .LVU31
.LLST3:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 .LVL11-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0
.LVUS4:
	.uleb128 .LVU18
	.uleb128 .LVU22
	.uleb128 .LVU22
	.uleb128 .LVU31
	.uleb128 .LVU31
	.uleb128 0
.LLST4:
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL7-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL7-1-.Ltext0
	.uleb128 .LVL11-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL11-.Ltext0
	.uleb128 .LFE23-.Ltext0
	.uleb128 0x8
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x71
	.sleb128 0
	.byte	0x22
	.byte	0x23
	.uleb128 0x30
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
.LASF10:
	.string	"signed char"
.LASF26:
	.string	"_cgo_errno"
.LASF12:
	.string	"long long unsigned int"
.LASF8:
	.string	"unsigned char"
.LASF3:
	.string	"long unsigned int"
.LASF29:
	.string	"getnameinfo"
.LASF9:
	.string	"short unsigned int"
.LASF20:
	.string	"__pad12"
.LASF15:
	.string	"sa_family_t"
.LASF30:
	.string	"_cgo_77133bf98b3a_Cfunc_getnameinfo"
.LASF22:
	.string	"__pad52"
.LASF23:
	.string	"_cgo_a"
.LASF4:
	.string	"unsigned int"
.LASF24:
	.string	"_cgo_stktop"
.LASF18:
	.string	"__errno_location"
.LASF25:
	.string	"_cgo_r"
.LASF13:
	.string	"__socklen_t"
.LASF5:
	.string	"long long int"
.LASF16:
	.string	"sa_family"
.LASF7:
	.string	"char"
.LASF27:
	.string	"GNU C17 11.4.0"
.LASF11:
	.string	"short int"
.LASF31:
	.string	"_cgo_77133bf98b3a_C2func_getnameinfo"
.LASF19:
	.string	"_cgo_topofstack"
.LASF2:
	.string	"long int"
.LASF14:
	.string	"socklen_t"
.LASF6:
	.string	"long double"
.LASF17:
	.string	"sa_data"
.LASF28:
	.string	"sockaddr"
.LASF21:
	.string	"__pad28"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/tmp/go-build"
.LASF0:
	.string	"cgo_resnew.cgo2.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
