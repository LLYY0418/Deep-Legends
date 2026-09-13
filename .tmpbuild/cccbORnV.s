	.arch armv8-a
	.file	"gcc_context.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_context.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_release_context
	.type	_cgo_release_context, %function
_cgo_release_context:
.LVL0:
.LFB39:
	.file 1 "gcc_context.c"
	.loc 1 10 43 view -0
	.cfi_startproc
	.loc 1 11 2 view .LVU1
	.loc 1 13 2 view .LVU2
	.loc 1 10 43 is_stmt 0 view .LVU3
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -32
	.loc 1 10 43 view .LVU4
	mov	x19, x0
	.loc 1 13 8 view .LVU5
	bl	_cgo_get_context_function
.LVL1:
	.loc 1 14 16 view .LVU6
	cmp	x19, 0
.LVL2:
	.loc 1 14 2 is_stmt 1 view .LVU7
	.loc 1 14 5 is_stmt 0 view .LVU8
	ccmp	x0, 0, 4, ne
	beq	.L1
.LBB2:
	.loc 1 15 3 is_stmt 1 view .LVU9
	.loc 1 17 3 view .LVU10
	.loc 1 17 15 is_stmt 0 view .LVU11
	str	x19, [sp, 40]
	.loc 1 18 3 is_stmt 1 view .LVU12
	mov	x1, x0
	.loc 1 18 4 is_stmt 0 view .LVU13
	add	x0, sp, 40
.LVL3:
	.loc 1 18 4 view .LVU14
	blr	x1
.LVL4:
.L1:
	.loc 1 18 4 view .LVU15
.LBE2:
	.loc 1 20 1 view .LVU16
	ldr	x19, [sp, 16]
.LVL5:
	.loc 1 20 1 view .LVU17
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE39:
	.size	_cgo_release_context, .-_cgo_release_context
.Letext0:
	.file 2 "/usr/include/stdint.h"
	.file 3 "libcgo.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x145
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x3
	.4byte	.LASF12
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF2
	.uleb128 0x1
	.byte	0x2
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x1
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF5
	.uleb128 0x1
	.byte	0x1
	.byte	0x6
	.4byte	.LASF6
	.uleb128 0x1
	.byte	0x2
	.byte	0x5
	.4byte	.LASF7
	.uleb128 0x4
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF9
	.uleb128 0x5
	.4byte	.LASF13
	.byte	0x2
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF10
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF11
	.uleb128 0x6
	.4byte	.LASF14
	.byte	0x8
	.byte	0x3
	.byte	0x5e
	.byte	0x8
	.4byte	0xa2
	.uleb128 0x7
	.4byte	.LASF15
	.byte	0x3
	.byte	0x5f
	.byte	0xc
	.4byte	0x6d
	.byte	0
	.byte	0
	.uleb128 0x8
	.4byte	0xad
	.uleb128 0x9
	.4byte	0xad
	.byte	0
	.uleb128 0x2
	.4byte	0x87
	.uleb128 0xa
	.4byte	.LASF16
	.byte	0x3
	.byte	0x61
	.byte	0x10
	.4byte	0xbe
	.uleb128 0x2
	.4byte	0xa2
	.uleb128 0xb
	.4byte	.LASF17
	.byte	0x1
	.byte	0xa
	.byte	0x6
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0xc
	.4byte	.LASF18
	.byte	0x1
	.byte	0xa
	.byte	0x25
	.4byte	0x6d
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0xd
	.string	"pfn"
	.byte	0x1
	.byte	0xb
	.byte	0x9
	.4byte	0xbe
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0xe
	.8byte	.LBB2
	.8byte	.LBE2-.LBB2
	.4byte	0x13a
	.uleb128 0xf
	.string	"arg"
	.byte	0x1
	.byte	0xf
	.byte	0x16
	.4byte	0x87
	.uleb128 0x2
	.byte	0x91
	.sleb128 -8
	.uleb128 0x10
	.8byte	.LVL4
	.uleb128 0x11
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x91
	.sleb128 -8
	.byte	0
	.byte	0
	.uleb128 0x12
	.8byte	.LVL1
	.4byte	0xb2
	.byte	0
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
	.uleb128 0x4
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
	.uleb128 0x5
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
	.uleb128 0x6
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
	.uleb128 0x7
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
	.uleb128 0x8
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x9
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xa
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
	.byte	0
	.byte	0
	.uleb128 0xc
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
	.uleb128 0xd
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
	.uleb128 0xe
	.uleb128 0xb
	.byte	0x1
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xf
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
	.uleb128 0x10
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.byte	0
	.byte	0
	.uleb128 0x11
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x12
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
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
	.uleb128 .LVU17
	.uleb128 .LVU17
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU7
	.uleb128 .LVU14
	.uleb128 .LVU14
	.uleb128 .LVU15
.LLST1:
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
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
	.string	"long long int"
.LASF4:
	.string	"unsigned int"
.LASF14:
	.string	"context_arg"
.LASF16:
	.string	"_cgo_get_context_function"
.LASF17:
	.string	"_cgo_release_context"
.LASF13:
	.string	"uintptr_t"
.LASF5:
	.string	"long unsigned int"
.LASF11:
	.string	"long long unsigned int"
.LASF2:
	.string	"unsigned char"
.LASF9:
	.string	"char"
.LASF8:
	.string	"long int"
.LASF7:
	.string	"short int"
.LASF12:
	.string	"GNU C17 11.4.0"
.LASF3:
	.string	"short unsigned int"
.LASF6:
	.string	"signed char"
.LASF15:
	.string	"Context"
.LASF18:
	.string	"ctxt"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_context.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
