	.arch armv8-a
	.file	"gcc_setenv.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_setenv.c"
	.align	2
	.p2align 4,,11
	.global	x_cgo_setenv
	.type	x_cgo_setenv, %function
x_cgo_setenv:
.LVL0:
.LFB39:
	.file 1 "gcc_setenv.c"
	.loc 1 14 1 view -0
	.cfi_startproc
	.loc 1 15 21 view .LVU1
	.loc 1 16 2 view .LVU2
	ldp	x0, x1, [x0]
.LVL1:
	.loc 1 16 2 is_stmt 0 view .LVU3
	mov	w2, 1
	b	setenv
.LVL2:
	.cfi_endproc
.LFE39:
	.size	x_cgo_setenv, .-x_cgo_setenv
	.align	2
	.p2align 4,,11
	.global	x_cgo_unsetenv
	.type	x_cgo_unsetenv, %function
x_cgo_unsetenv:
.LVL3:
.LFB40:
	.loc 1 23 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 24 21 view .LVU5
	.loc 1 25 2 view .LVU6
	ldr	x0, [x0]
.LVL4:
	.loc 1 25 2 is_stmt 0 view .LVU7
	b	unsetenv
.LVL5:
	.cfi_endproc
.LFE40:
	.size	x_cgo_unsetenv, .-x_cgo_unsetenv
.Letext0:
	.file 2 "/usr/include/stdlib.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x140
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x6
	.4byte	.LASF14
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
	.uleb128 0x7
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x3
	.4byte	0x6b
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF9
	.uleb128 0x8
	.4byte	0x6b
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF10
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF11
	.uleb128 0x4
	.4byte	.LASF12
	.2byte	0x298
	.4byte	0x58
	.4byte	0x9a
	.uleb128 0x2
	.4byte	0x9a
	.byte	0
	.uleb128 0x3
	.4byte	0x72
	.uleb128 0x4
	.4byte	.LASF13
	.2byte	0x294
	.4byte	0x58
	.4byte	0xbe
	.uleb128 0x2
	.4byte	0x9a
	.uleb128 0x2
	.4byte	0x9a
	.uleb128 0x2
	.4byte	0x58
	.byte	0
	.uleb128 0x9
	.4byte	.LASF15
	.byte	0x1
	.byte	0x16
	.byte	0x1
	.8byte	.LFB40
	.8byte	.LFE40-.LFB40
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xfd
	.uleb128 0x5
	.string	"arg"
	.byte	0x16
	.byte	0x17
	.4byte	0xfd
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0xa
	.8byte	.LVL5
	.4byte	0x85
	.byte	0
	.uleb128 0x3
	.4byte	0x66
	.uleb128 0xb
	.4byte	.LASF16
	.byte	0x1
	.byte	0xd
	.byte	0x1
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x5
	.string	"arg"
	.byte	0xd
	.byte	0x15
	.4byte	0xfd
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0xc
	.8byte	.LVL2
	.4byte	0x9f
	.uleb128 0xd
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.byte	0x31
	.byte	0
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
	.uleb128 0x5
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
	.uleb128 0x5
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
	.uleb128 0x6
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
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
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
	.uleb128 0xa
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
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x82
	.uleb128 0x19
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xd
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
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
.LVUS1:
	.uleb128 0
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LFE40-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS0:
	.uleb128 0
	.uleb128 .LVU3
	.uleb128 .LVU3
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
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
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
.LASF12:
	.string	"unsetenv"
.LASF14:
	.string	"GNU C17 11.4.0"
.LASF5:
	.string	"long unsigned int"
.LASF15:
	.string	"x_cgo_unsetenv"
.LASF11:
	.string	"long long unsigned int"
.LASF13:
	.string	"setenv"
.LASF16:
	.string	"x_cgo_setenv"
.LASF2:
	.string	"unsigned char"
.LASF9:
	.string	"char"
.LASF8:
	.string	"long int"
.LASF3:
	.string	"short unsigned int"
.LASF6:
	.string	"signed char"
.LASF7:
	.string	"short int"
	.section	.debug_line_str,"MS",@progbits,1
.LASF0:
	.string	"gcc_setenv.c"
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
