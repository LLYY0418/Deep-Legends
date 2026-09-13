	.arch armv8-a
	.file	"gcc_stack_unix.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_stack_unix.c"
	.align	2
	.p2align 4,,11
	.global	x_cgo_getstackbound
	.type	x_cgo_getstackbound, %function
x_cgo_getstackbound:
.LVL0:
.LFB47:
	.file 1 "gcc_stack_unix.c"
	.loc 1 16 1 view -0
	.cfi_startproc
	.loc 1 17 2 view .LVU1
	.loc 1 18 2 view .LVU2
	.loc 1 19 2 view .LVU3
	.loc 1 23 2 view .LVU4
	.loc 1 16 1 is_stmt 0 view .LVU5
	stp	x29, x30, [sp, -112]!
	.cfi_def_cfa_offset 112
	.cfi_offset 29, -112
	.cfi_offset 30, -104
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -96
	.cfi_offset 20, -88
	.loc 1 23 2 view .LVU6
	add	x19, sp, 48
	.loc 1 16 1 view .LVU7
	mov	x20, x0
	.loc 1 23 2 view .LVU8
	mov	x0, x19
.LVL1:
	.loc 1 23 2 view .LVU9
	bl	pthread_attr_init
.LVL2:
	.loc 1 28 2 is_stmt 1 view .LVU10
	bl	pthread_self
.LVL3:
	mov	x1, x19
	bl	pthread_getattr_np
.LVL4:
	.loc 1 29 2 view .LVU11
	add	x2, sp, 40
	add	x1, sp, 32
	mov	x0, x19
	bl	pthread_attr_getstack
.LVL5:
	.loc 1 40 2 view .LVU12
	mov	x0, x19
	bl	pthread_attr_destroy
.LVL6:
	.loc 1 44 21 view .LVU13
	.loc 1 45 2 view .LVU14
	.loc 1 46 28 is_stmt 0 view .LVU15
	ldp	x1, x0, [sp, 32]
	add	x0, x0, x1
	.loc 1 46 12 view .LVU16
	stp	x1, x0, [x20]
	.loc 1 47 21 is_stmt 1 view .LVU17
	.loc 1 48 1 is_stmt 0 view .LVU18
	ldp	x19, x20, [sp, 16]
.LVL7:
	.loc 1 48 1 view .LVU19
	ldp	x29, x30, [sp], 112
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE47:
	.size	x_cgo_getstackbound, .-x_cgo_getstackbound
.Letext0:
	.file 2 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 3 "/usr/include/aarch64-linux-gnu/bits/pthreadtypes.h"
	.file 4 "/usr/include/stdint.h"
	.file 5 "libcgo.h"
	.file 6 "/usr/include/pthread.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x259
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0xb
	.4byte	.LASF26
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
	.uleb128 0xc
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0xd
	.byte	0x8
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF9
	.uleb128 0x4
	.4byte	.LASF11
	.byte	0x2
	.byte	0xd1
	.byte	0x17
	.4byte	0x43
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF10
	.uleb128 0x4
	.4byte	.LASF12
	.byte	0x3
	.byte	0x1b
	.byte	0x1b
	.4byte	0x43
	.uleb128 0xe
	.4byte	.LASF15
	.byte	0x40
	.byte	0x3
	.byte	0x38
	.byte	0x7
	.4byte	0xb2
	.uleb128 0xa
	.4byte	.LASF13
	.byte	0x3a
	.byte	0x8
	.4byte	0xb2
	.uleb128 0xa
	.4byte	.LASF14
	.byte	0x3b
	.byte	0xc
	.4byte	0x5f
	.byte	0
	.uleb128 0xf
	.4byte	0x68
	.4byte	0xc2
	.uleb128 0x10
	.4byte	0x43
	.byte	0x3f
	.byte	0
	.uleb128 0x4
	.4byte	.LASF15
	.byte	0x3
	.byte	0x3e
	.byte	0x1e
	.4byte	0x8e
	.uleb128 0x11
	.4byte	0xc2
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF16
	.uleb128 0x4
	.4byte	.LASF17
	.byte	0x4
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x4
	.4byte	.LASF18
	.byte	0x5
	.byte	0xf
	.byte	0x13
	.4byte	0xda
	.uleb128 0x5
	.4byte	0xe6
	.uleb128 0x6
	.4byte	.LASF19
	.2byte	0x120
	.4byte	0x58
	.4byte	0x10c
	.uleb128 0x2
	.4byte	0x10c
	.byte	0
	.uleb128 0x5
	.4byte	0xc2
	.uleb128 0x6
	.4byte	.LASF20
	.2byte	0x17b
	.4byte	0x58
	.4byte	0x130
	.uleb128 0x2
	.4byte	0x135
	.uleb128 0x2
	.4byte	0x13f
	.uleb128 0x2
	.4byte	0x149
	.byte	0
	.uleb128 0x5
	.4byte	0xce
	.uleb128 0x7
	.4byte	0x130
	.uleb128 0x5
	.4byte	0x66
	.uleb128 0x7
	.4byte	0x13a
	.uleb128 0x5
	.4byte	0x6f
	.uleb128 0x7
	.4byte	0x144
	.uleb128 0x6
	.4byte	.LASF21
	.2byte	0x1b0
	.4byte	0x58
	.4byte	0x168
	.uleb128 0x2
	.4byte	0x82
	.uleb128 0x2
	.4byte	0x10c
	.byte	0
	.uleb128 0x12
	.4byte	.LASF27
	.byte	0x6
	.2byte	0x111
	.byte	0x12
	.4byte	0x82
	.uleb128 0x6
	.4byte	.LASF22
	.2byte	0x11d
	.4byte	0x58
	.4byte	0x18a
	.uleb128 0x2
	.4byte	0x10c
	.byte	0
	.uleb128 0x13
	.4byte	.LASF28
	.byte	0x1
	.byte	0xf
	.byte	0x1
	.8byte	.LFB47
	.8byte	.LFE47-.LFB47
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x14
	.4byte	.LASF29
	.byte	0x1
	.byte	0xf
	.byte	0x1d
	.4byte	0xf2
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x8
	.4byte	.LASF23
	.byte	0x11
	.byte	0x11
	.4byte	0xc2
	.uleb128 0x2
	.byte	0x91
	.sleb128 -64
	.uleb128 0x8
	.4byte	.LASF24
	.byte	0x12
	.byte	0x8
	.4byte	0x66
	.uleb128 0x3
	.byte	0x91
	.sleb128 -80
	.uleb128 0x8
	.4byte	.LASF25
	.byte	0x13
	.byte	0x9
	.4byte	0x6f
	.uleb128 0x3
	.byte	0x91
	.sleb128 -72
	.uleb128 0x9
	.8byte	.LVL2
	.4byte	0x175
	.4byte	0x1fc
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x15
	.8byte	.LVL3
	.4byte	0x168
	.uleb128 0x9
	.8byte	.LVL4
	.4byte	0x14e
	.4byte	0x221
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x9
	.8byte	.LVL5
	.4byte	0x111
	.4byte	0x247
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0x91
	.sleb128 -80
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x3
	.byte	0x91
	.sleb128 -72
	.byte	0
	.uleb128 0x16
	.8byte	.LVL6
	.4byte	0xf7
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
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
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
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
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x6
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 6
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
	.uleb128 0x7
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
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
	.uleb128 0xa
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
	.uleb128 0xb
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
	.uleb128 0xc
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
	.uleb128 0xd
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xe
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
	.uleb128 0xf
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x11
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x12
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
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
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
	.uleb128 0x14
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
	.uleb128 0x15
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x16
	.uleb128 0x48
	.byte	0x1
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
	.uleb128 .LVU9
	.uleb128 .LVU9
	.uleb128 .LVU19
	.uleb128 .LVU19
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL7-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LFE47-.Ltext0
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
.LASF16:
	.string	"long long int"
.LASF3:
	.string	"short unsigned int"
.LASF4:
	.string	"unsigned int"
.LASF11:
	.string	"size_t"
.LASF15:
	.string	"pthread_attr_t"
.LASF29:
	.string	"bounds"
.LASF26:
	.string	"GNU C17 11.4.0"
.LASF5:
	.string	"long unsigned int"
.LASF22:
	.string	"pthread_attr_init"
.LASF24:
	.string	"addr"
.LASF23:
	.string	"attr"
.LASF28:
	.string	"x_cgo_getstackbound"
.LASF10:
	.string	"long long unsigned int"
.LASF2:
	.string	"unsigned char"
.LASF9:
	.string	"char"
.LASF18:
	.string	"uintptr"
.LASF8:
	.string	"long int"
.LASF13:
	.string	"__size"
.LASF21:
	.string	"pthread_getattr_np"
.LASF20:
	.string	"pthread_attr_getstack"
.LASF25:
	.string	"size"
.LASF19:
	.string	"pthread_attr_destroy"
.LASF27:
	.string	"pthread_self"
.LASF17:
	.string	"uintptr_t"
.LASF14:
	.string	"__align"
.LASF7:
	.string	"short int"
.LASF6:
	.string	"signed char"
.LASF12:
	.string	"pthread_t"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_stack_unix.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
