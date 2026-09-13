	.arch armv8-a
	.file	"_cgo_export.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT-build" "_cgo_export.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc__Cmalloc
	.type	_cgo_77133bf98b3a_Cfunc__Cmalloc, %function
_cgo_77133bf98b3a_Cfunc__Cmalloc:
.LVL0:
.LFB16:
	.file 1 "_cgo_export.c"
	.loc 1 25 48 view -0
	.cfi_startproc
	.loc 1 26 2 view .LVU1
	.loc 1 25 48 is_stmt 0 view .LVU2
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -16
	.cfi_offset 20, -8
	.loc 1 25 48 view .LVU3
	mov	x19, x0
.LVL1:
	.loc 1 30 2 is_stmt 1 view .LVU4
	.loc 1 31 21 view .LVU5
	.loc 1 32 2 view .LVU6
	.loc 1 32 16 is_stmt 0 view .LVU7
	ldr	x20, [x0]
	.loc 1 32 8 view .LVU8
	mov	x0, x20
.LVL2:
	.loc 1 32 8 view .LVU9
	bl	malloc
.LVL3:
	.loc 1 33 2 is_stmt 1 view .LVU10
	.loc 1 33 5 is_stmt 0 view .LVU11
	cbz	x0, .L5
.L2:
	.loc 1 36 2 is_stmt 1 view .LVU12
	.loc 1 36 8 is_stmt 0 view .LVU13
	str	x0, [x19, 8]
	.loc 1 37 21 is_stmt 1 view .LVU14
	.loc 1 38 1 is_stmt 0 view .LVU15
	ldp	x19, x20, [sp, 16]
.LVL4:
	.loc 1 38 1 view .LVU16
	ldp	x29, x30, [sp], 32
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL5:
.L5:
	.cfi_restore_state
	.loc 1 33 15 discriminator 1 view .LVU17
	cbnz	x20, .L2
	.loc 1 34 3 is_stmt 1 view .LVU18
	.loc 1 34 9 is_stmt 0 view .LVU19
	mov	x0, 1
.LVL6:
	.loc 1 34 9 view .LVU20
	bl	malloc
.LVL7:
	.loc 1 34 9 view .LVU21
	b	.L2
	.cfi_endproc
.LFE16:
	.size	_cgo_77133bf98b3a_Cfunc__Cmalloc, .-_cgo_77133bf98b3a_Cfunc__Cmalloc
.Letext0:
	.file 2 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 3 "/usr/include/stdlib.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x166
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x5
	.4byte	.LASF17
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x6
	.4byte	.LASF18
	.byte	0x2
	.byte	0xd1
	.byte	0x17
	.4byte	0x3a
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF2
	.uleb128 0x1
	.byte	0x4
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x7
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF4
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF5
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF6
	.uleb128 0x1
	.byte	0x2
	.byte	0x7
	.4byte	.LASF7
	.uleb128 0x1
	.byte	0x1
	.byte	0x6
	.4byte	.LASF8
	.uleb128 0x1
	.byte	0x2
	.byte	0x5
	.4byte	.LASF9
	.uleb128 0x8
	.byte	0x8
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF10
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF11
	.uleb128 0x1
	.byte	0x10
	.byte	0x4
	.4byte	.LASF12
	.uleb128 0x1
	.byte	0x4
	.byte	0x4
	.4byte	.LASF13
	.uleb128 0x1
	.byte	0x8
	.byte	0x4
	.4byte	.LASF14
	.uleb128 0x1
	.byte	0x8
	.byte	0x3
	.4byte	.LASF15
	.uleb128 0x1
	.byte	0x10
	.byte	0x3
	.4byte	.LASF16
	.uleb128 0x9
	.4byte	.LASF19
	.byte	0x3
	.2byte	0x21c
	.byte	0xe
	.4byte	0x79
	.4byte	0xc3
	.uleb128 0xa
	.4byte	0x2e
	.byte	0
	.uleb128 0xb
	.4byte	.LASF20
	.byte	0x1
	.byte	0x19
	.byte	0x6
	.8byte	.LFB16
	.8byte	.LFE16-.LFB16
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x163
	.uleb128 0xc
	.string	"v"
	.byte	0x1
	.byte	0x19
	.byte	0x2d
	.4byte	0x79
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0xd
	.byte	0x10
	.byte	0x1
	.byte	0x1a
	.byte	0x2
	.4byte	0x113
	.uleb128 0x2
	.string	"p0"
	.byte	0x1b
	.byte	0x16
	.4byte	0x82
	.byte	0
	.uleb128 0x2
	.string	"r1"
	.byte	0x1c
	.byte	0x9
	.4byte	0x79
	.byte	0x8
	.byte	0
	.uleb128 0x3
	.string	"a"
	.byte	0x1d
	.byte	0x21
	.4byte	0x163
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x3
	.string	"ret"
	.byte	0x1e
	.byte	0x8
	.4byte	0x79
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0xe
	.8byte	.LVL3
	.4byte	0xac
	.4byte	0x14f
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.uleb128 0xf
	.8byte	.LVL7
	.4byte	0xac
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x31
	.byte	0
	.byte	0
	.uleb128 0x10
	.byte	0x8
	.4byte	0xf3
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
	.uleb128 0x4
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x5
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
	.uleb128 0x8
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x9
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
	.uleb128 0xa
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xb
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
	.uleb128 0xc
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
	.uleb128 0xd
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
	.uleb128 0xe
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
	.uleb128 0xf
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x49
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
	.uleb128 .LVU9
	.uleb128 .LVU9
	.uleb128 .LVU16
	.uleb128 .LVU16
	.uleb128 .LVU17
	.uleb128 .LVU17
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE16-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS1:
	.uleb128 .LVU4
	.uleb128 .LVU9
	.uleb128 .LVU9
	.uleb128 .LVU16
	.uleb128 .LVU16
	.uleb128 .LVU17
	.uleb128 .LVU17
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE16-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS2:
	.uleb128 .LVU10
	.uleb128 .LVU20
	.uleb128 .LVU21
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LFE16-.Ltext0
	.uleb128 0x1
	.byte	0x50
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
.LASF6:
	.string	"unsigned char"
.LASF18:
	.string	"size_t"
.LASF16:
	.string	"complex double"
.LASF2:
	.string	"long unsigned int"
.LASF11:
	.string	"long long unsigned int"
.LASF15:
	.string	"complex float"
.LASF20:
	.string	"_cgo_77133bf98b3a_Cfunc__Cmalloc"
.LASF13:
	.string	"float"
.LASF10:
	.string	"char"
.LASF4:
	.string	"long int"
.LASF14:
	.string	"double"
.LASF17:
	.string	"GNU C17 11.4.0"
.LASF7:
	.string	"short unsigned int"
.LASF8:
	.string	"signed char"
.LASF12:
	.string	"long double"
.LASF9:
	.string	"short int"
.LASF3:
	.string	"unsigned int"
.LASF19:
	.string	"malloc"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/tmp/go-build"
.LASF0:
	.string	"_cgo_export.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
