	.arch armv8-a
	.file	"gcc_traceback.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_traceback.c"
	.align	2
	.p2align 4,,11
	.global	x_cgo_callers
	.type	x_cgo_callers, %function
x_cgo_callers:
.LVL0:
.LFB39:
	.file 1 "gcc_traceback.c"
	.loc 1 23 170 view -0
	.cfi_startproc
	.loc 1 24 2 view .LVU1
	.loc 1 26 2 view .LVU2
	.loc 1 23 170 is_stmt 0 view .LVU3
	stp	x29, x30, [sp, -80]!
	.cfi_def_cfa_offset 80
	.cfi_offset 29, -80
	.cfi_offset 30, -72
	mov	x29, sp
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -48
	.cfi_offset 22, -40
	mov	x22, x1
	.loc 1 29 10 view .LVU4
	mov	x1, 32
.LVL1:
	.loc 1 23 170 view .LVU5
	mov	x21, x0
	.loc 1 43 3 view .LVU6
	add	x0, sp, 48
.LVL2:
	.loc 1 23 170 view .LVU7
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -64
	.cfi_offset 20, -56
	.loc 1 23 170 view .LVU8
	mov	x20, x5
	mov	x19, x2
	.loc 1 27 17 view .LVU9
	stp	xzr, x2, [sp, 48]
	.loc 1 28 2 is_stmt 1 view .LVU10
	.loc 1 29 10 is_stmt 0 view .LVU11
	stp	x4, x1, [sp, 64]
	.loc 1 42 21 is_stmt 1 view .LVU12
	.loc 1 43 2 view .LVU13
	.loc 1 43 3 is_stmt 0 view .LVU14
	blr	x3
.LVL3:
	.loc 1 44 21 is_stmt 1 view .LVU15
	.loc 1 45 2 view .LVU16
	mov	x2, x19
	mov	x1, x22
	mov	x0, x21
	blr	x20
.LVL4:
	.loc 1 46 1 is_stmt 0 view .LVU17
	ldp	x19, x20, [sp, 16]
.LVL5:
	.loc 1 46 1 view .LVU18
	ldp	x21, x22, [sp, 32]
.LVL6:
	.loc 1 46 1 view .LVU19
	ldp	x29, x30, [sp], 80
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE39:
	.size	x_cgo_callers, .-x_cgo_callers
.Letext0:
	.file 2 "libcgo.h"
	.file 3 "/usr/include/stdint.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x1c7
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x9
	.4byte	.LASF19
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
	.uleb128 0xa
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0xb
	.byte	0x8
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF9
	.uleb128 0xc
	.4byte	.LASF20
	.byte	0x3
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
	.uleb128 0x3
	.4byte	0x6f
	.uleb128 0xd
	.4byte	.LASF21
	.byte	0x20
	.byte	0x2
	.byte	0x66
	.byte	0x8
	.4byte	0xc8
	.uleb128 0x6
	.4byte	.LASF12
	.byte	0x67
	.4byte	0x6f
	.byte	0
	.uleb128 0x6
	.4byte	.LASF13
	.byte	0x68
	.4byte	0x6f
	.byte	0x8
	.uleb128 0x7
	.string	"Buf"
	.byte	0x69
	.4byte	0x89
	.byte	0x10
	.uleb128 0x7
	.string	"Max"
	.byte	0x6a
	.4byte	0x6f
	.byte	0x18
	.byte	0
	.uleb128 0xe
	.4byte	.LASF22
	.byte	0x1
	.byte	0x17
	.byte	0x1
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x19b
	.uleb128 0xf
	.string	"sig"
	.byte	0x1
	.byte	0x17
	.byte	0x19
	.4byte	0x6f
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x2
	.4byte	.LASF14
	.byte	0x24
	.4byte	0x66
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x2
	.4byte	.LASF15
	.byte	0x30
	.4byte	0x66
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x2
	.4byte	.LASF16
	.byte	0x40
	.4byte	0x1ab
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x2
	.4byte	.LASF17
	.byte	0x73
	.4byte	0x89
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x2
	.4byte	.LASF18
	.byte	0x86
	.4byte	0x1c5
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0x10
	.string	"arg"
	.byte	0x1
	.byte	0x18
	.byte	0x19
	.4byte	0x8e
	.uleb128 0x2
	.byte	0x91
	.sleb128 -32
	.uleb128 0x11
	.8byte	.LVL3
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.4byte	0x17b
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x91
	.sleb128 -32
	.byte	0
	.uleb128 0x12
	.8byte	.LVL4
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x85
	.sleb128 0
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x86
	.sleb128 0
	.uleb128 0x4
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.byte	0
	.uleb128 0x8
	.4byte	0x1a6
	.uleb128 0x5
	.4byte	0x1a6
	.byte	0
	.uleb128 0x3
	.4byte	0x8e
	.uleb128 0x3
	.4byte	0x19b
	.uleb128 0x8
	.4byte	0x1c5
	.uleb128 0x5
	.4byte	0x6f
	.uleb128 0x5
	.4byte	0x66
	.uleb128 0x5
	.4byte	0x66
	.byte	0
	.uleb128 0x3
	.4byte	0x1b0
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
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 23
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
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
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
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 13
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x7
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 13
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
	.uleb128 0xd
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
	.uleb128 0xf
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
	.uleb128 0x10
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
	.uleb128 0x11
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x83
	.uleb128 0x18
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x12
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x83
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
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU19
	.uleb128 .LVU19
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 0
	.uleb128 .LVU5
	.uleb128 .LVU5
	.uleb128 .LVU19
	.uleb128 .LVU19
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU15
	.uleb128 .LVU15
	.uleb128 .LVU18
	.uleb128 .LVU18
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 0x1
	.byte	0x52
	.byte	0x4
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0
.LVUS3:
	.uleb128 0
	.uleb128 .LVU15
	.uleb128 .LVU15
	.uleb128 0
.LLST3:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 0x1
	.byte	0x53
	.byte	0x4
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0x9f
	.byte	0
.LVUS4:
	.uleb128 0
	.uleb128 .LVU15
	.uleb128 .LVU15
	.uleb128 0
.LLST4:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 0x1
	.byte	0x54
	.byte	0x4
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x54
	.byte	0x9f
	.byte	0
.LVUS5:
	.uleb128 0
	.uleb128 .LVU15
	.uleb128 .LVU15
	.uleb128 .LVU18
	.uleb128 .LVU18
	.uleb128 0
.LLST5:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 0x1
	.byte	0x55
	.byte	0x4
	.uleb128 .LVL3-1-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x55
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
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF10:
	.string	"long long int"
.LASF4:
	.string	"unsigned int"
.LASF20:
	.string	"uintptr_t"
.LASF5:
	.string	"long unsigned int"
.LASF11:
	.string	"long long unsigned int"
.LASF17:
	.string	"cgoCallers"
.LASF22:
	.string	"x_cgo_callers"
.LASF14:
	.string	"info"
.LASF16:
	.string	"cgoTraceback"
.LASF2:
	.string	"unsigned char"
.LASF9:
	.string	"char"
.LASF8:
	.string	"long int"
.LASF15:
	.string	"context"
.LASF13:
	.string	"SigContext"
.LASF19:
	.string	"GNU C17 11.4.0"
.LASF21:
	.string	"cgoTracebackArg"
.LASF3:
	.string	"short unsigned int"
.LASF6:
	.string	"signed char"
.LASF12:
	.string	"Context"
.LASF18:
	.string	"sigtramp"
.LASF7:
	.string	"short int"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_traceback.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
