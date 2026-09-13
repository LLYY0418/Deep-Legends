	.arch armv8-a
	.file	"cgo_unix_cgo_res.cgo2.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT-build" "cgo_unix_cgo_res.cgo2.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_res_search
	.type	_cgo_77133bf98b3a_Cfunc_res_search, %function
_cgo_77133bf98b3a_Cfunc_res_search:
.LVL0:
.LFB58:
	.file 1 "cgo-gcc-prolog"
	.loc 1 46 1 view -0
	.cfi_startproc
	.loc 1 47 2 view .LVU1
	.loc 1 46 1 is_stmt 0 view .LVU2
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
	.loc 1 57 2 is_stmt 1 view .LVU3
	.loc 1 46 1 is_stmt 0 view .LVU4
	str	x21, [sp, 32]
	.cfi_offset 21, -16
	.loc 1 57 22 view .LVU5
	bl	_cgo_topofstack
.LVL2:
	.loc 1 60 11 view .LVU6
	ldp	w1, w2, [x19, 8]
	.loc 1 57 22 view .LVU7
	mov	x21, x0
.LVL3:
	.loc 1 58 2 is_stmt 1 view .LVU8
	.loc 1 59 21 view .LVU9
	.loc 1 60 2 view .LVU10
	.loc 1 60 11 is_stmt 0 view .LVU11
	ldr	w4, [x19, 24]
	ldr	x0, [x19]
.LVL4:
	.loc 1 60 11 view .LVU12
	ldr	x3, [x19, 16]
	bl	res_search
.LVL5:
	mov	w20, w0
.LVL6:
	.loc 1 61 21 is_stmt 1 view .LVU13
	.loc 1 62 2 view .LVU14
	.loc 1 62 36 is_stmt 0 view .LVU15
	bl	_cgo_topofstack
.LVL7:
	.loc 1 63 2 is_stmt 1 view .LVU16
	.loc 1 62 54 is_stmt 0 view .LVU17
	sub	x0, x0, x21
.LVL8:
	.loc 1 63 12 view .LVU18
	add	x19, x19, x0
.LVL9:
	.loc 1 65 1 view .LVU19
	ldr	x21, [sp, 32]
.LVL10:
	.loc 1 63 12 view .LVU20
	str	w20, [x19, 32]
	.loc 1 64 48 is_stmt 1 view .LVU21
	.loc 1 65 1 is_stmt 0 view .LVU22
	ldp	x19, x20, [sp, 16]
.LVL11:
	.loc 1 65 1 view .LVU23
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE58:
	.size	_cgo_77133bf98b3a_Cfunc_res_search, .-_cgo_77133bf98b3a_Cfunc_res_search
.Letext0:
	.file 2 "/usr/include/resolv.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x1d9
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x8
	.4byte	.LASF19
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF2
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x1
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF5
	.uleb128 0x1
	.byte	0x10
	.byte	0x4
	.4byte	.LASF6
	.uleb128 0x4
	.4byte	0x5d
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF7
	.uleb128 0x9
	.4byte	0x56
	.uleb128 0x4
	.4byte	0x56
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF8
	.uleb128 0x1
	.byte	0x2
	.byte	0x7
	.4byte	.LASF9
	.uleb128 0x1
	.byte	0x1
	.byte	0x6
	.4byte	.LASF10
	.uleb128 0x1
	.byte	0x2
	.byte	0x5
	.4byte	.LASF11
	.uleb128 0xa
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0xb
	.byte	0x8
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF12
	.uleb128 0x1
	.byte	0x10
	.byte	0x7
	.4byte	.LASF13
	.uleb128 0x4
	.4byte	0x67
	.uleb128 0xc
	.4byte	0x56
	.4byte	0xaf
	.uleb128 0xd
	.4byte	0x35
	.byte	0x3
	.byte	0
	.uleb128 0xe
	.4byte	.LASF20
	.byte	0x2
	.byte	0xc8
	.byte	0x6
	.4byte	0x83
	.4byte	0xd9
	.uleb128 0x3
	.4byte	0x51
	.uleb128 0x3
	.4byte	0x83
	.uleb128 0x3
	.4byte	0x83
	.uleb128 0x3
	.4byte	0x9a
	.uleb128 0x3
	.4byte	0x83
	.byte	0
	.uleb128 0xf
	.4byte	.LASF21
	.byte	0x1
	.byte	0x13
	.byte	0xe
	.4byte	0x62
	.uleb128 0x10
	.4byte	.LASF22
	.byte	0x1
	.byte	0x2d
	.byte	0x1
	.8byte	.LFB58
	.8byte	.LFE58-.LFB58
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x1d7
	.uleb128 0x11
	.string	"v"
	.byte	0x1
	.byte	0x2d
	.byte	0x2a
	.4byte	0x8a
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x12
	.byte	0x28
	.byte	0x1
	.byte	0x2f
	.byte	0x2
	.4byte	0x176
	.uleb128 0x2
	.string	"p0"
	.byte	0x30
	.byte	0xf
	.4byte	0x51
	.byte	0
	.uleb128 0x2
	.string	"p1"
	.byte	0x31
	.byte	0x7
	.4byte	0x83
	.byte	0x8
	.uleb128 0x2
	.string	"p2"
	.byte	0x32
	.byte	0x7
	.4byte	0x83
	.byte	0xc
	.uleb128 0x2
	.string	"p3"
	.byte	0x33
	.byte	0x12
	.4byte	0x9a
	.byte	0x10
	.uleb128 0x2
	.string	"p4"
	.byte	0x34
	.byte	0x7
	.4byte	0x83
	.byte	0x18
	.uleb128 0x7
	.4byte	.LASF14
	.byte	0x35
	.4byte	0x9f
	.byte	0x1c
	.uleb128 0x2
	.string	"r"
	.byte	0x36
	.byte	0x7
	.4byte	0x83
	.byte	0x20
	.uleb128 0x7
	.4byte	.LASF15
	.byte	0x37
	.4byte	0x9f
	.byte	0x24
	.byte	0
	.uleb128 0x5
	.4byte	.LASF16
	.byte	0x38
	.byte	0x21
	.4byte	0x1d7
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x5
	.4byte	.LASF17
	.byte	0x39
	.byte	0x8
	.4byte	0x62
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x5
	.4byte	.LASF18
	.byte	0x3a
	.byte	0x18
	.4byte	0x83
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x6
	.8byte	.LVL2
	.4byte	0xd9
	.uleb128 0x6
	.8byte	.LVL5
	.4byte	0xaf
	.uleb128 0x6
	.8byte	.LVL7
	.4byte	0xd9
	.byte	0
	.uleb128 0x4
	.4byte	0x115
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
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
	.uleb128 0x2
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
	.uleb128 0x3
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x4
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
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
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x8
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
	.uleb128 0x9
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xa
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
	.uleb128 0xb
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
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
	.uleb128 0xf
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
	.uleb128 0x10
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
	.uleb128 0x11
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
	.uleb128 0x12
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
	.uleb128 .LVU6
	.uleb128 .LVU6
	.uleb128 .LVU19
	.uleb128 .LVU19
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
	.uleb128 .LFE58-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU3
	.uleb128 .LVU6
	.uleb128 .LVU6
	.uleb128 .LVU16
	.uleb128 .LVU16
	.uleb128 .LVU18
.LLST1:
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
	.byte	0x85
	.sleb128 0
	.byte	0x1c
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x9f
	.byte	0
.LVUS2:
	.uleb128 .LVU8
	.uleb128 .LVU12
	.uleb128 .LVU12
	.uleb128 .LVU20
.LLST2:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL10-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS3:
	.uleb128 .LVU13
	.uleb128 .LVU16
	.uleb128 .LVU16
	.uleb128 .LVU23
	.uleb128 .LVU23
	.uleb128 0
.LLST3:
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL7-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL7-1-.Ltext0
	.uleb128 .LVL11-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL11-.Ltext0
	.uleb128 .LFE58-.Ltext0
	.uleb128 0x8
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x23
	.uleb128 0x20
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
.LASF5:
	.string	"long long int"
.LASF4:
	.string	"unsigned int"
.LASF14:
	.string	"__pad28"
.LASF20:
	.string	"res_search"
.LASF15:
	.string	"__pad36"
.LASF19:
	.string	"GNU C17 11.4.0"
.LASF18:
	.string	"_cgo_r"
.LASF12:
	.string	"long long unsigned int"
.LASF9:
	.string	"short unsigned int"
.LASF8:
	.string	"unsigned char"
.LASF7:
	.string	"char"
.LASF2:
	.string	"long int"
.LASF3:
	.string	"long unsigned int"
.LASF13:
	.string	"__int128 unsigned"
.LASF21:
	.string	"_cgo_topofstack"
.LASF17:
	.string	"_cgo_stktop"
.LASF22:
	.string	"_cgo_77133bf98b3a_Cfunc_res_search"
.LASF10:
	.string	"signed char"
.LASF6:
	.string	"long double"
.LASF11:
	.string	"short int"
.LASF16:
	.string	"_cgo_a"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/tmp/go-build"
.LASF0:
	.string	"cgo_unix_cgo_res.cgo2.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
