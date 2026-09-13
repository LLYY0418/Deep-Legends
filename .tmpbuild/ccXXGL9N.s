	.arch armv8-a
	.file	"_cgo_main.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT-build" "_cgo_main.c"
	.section	.text.startup,"ax",@progbits
	.align	2
	.p2align 4,,11
	.global	main
	.type	main, %function
main:
.LVL0:
.LFB0:
	.file 1 "_cgo_main.c"
	.loc 1 2 81 view -0
	.cfi_startproc
	.loc 1 2 83 view .LVU1
	.loc 1 2 93 is_stmt 0 view .LVU2
	mov	w0, 0
.LVL1:
	.loc 1 2 93 view .LVU3
	ret
	.cfi_endproc
.LFE0:
	.size	main, .-main
	.text
	.align	2
	.p2align 4,,11
	.global	crosscall2
	.type	crosscall2, %function
crosscall2:
.LVL2:
.LFB1:
	.loc 1 3 160 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 3 162 view .LVU5
	ret
	.cfi_endproc
.LFE1:
	.size	crosscall2, .-crosscall2
	.align	2
	.p2align 4,,11
	.global	_cgo_wait_runtime_init_done
	.type	_cgo_wait_runtime_init_done, %function
_cgo_wait_runtime_init_done:
.LFB2:
	.loc 1 4 42 view -0
	.cfi_startproc
	.loc 1 4 44 view .LVU7
	.loc 1 4 54 is_stmt 0 view .LVU8
	mov	x0, 0
	ret
	.cfi_endproc
.LFE2:
	.size	_cgo_wait_runtime_init_done, .-_cgo_wait_runtime_init_done
	.align	2
	.p2align 4,,11
	.global	_cgo_release_context
	.type	_cgo_release_context, %function
_cgo_release_context:
.LVL3:
.LFB3:
	.loc 1 5 64 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 5 66 view .LVU10
	ret
	.cfi_endproc
.LFE3:
	.size	_cgo_release_context, .-_cgo_release_context
	.align	2
	.p2align 4,,11
	.global	_cgo_topofstack
	.type	_cgo_topofstack, %function
_cgo_topofstack:
.LFB4:
	.loc 1 6 29 view -0
	.cfi_startproc
	.loc 1 6 31 view .LVU12
	.loc 1 6 48 is_stmt 0 view .LVU13
	mov	x0, 0
	ret
	.cfi_endproc
.LFE4:
	.size	_cgo_topofstack, .-_cgo_topofstack
	.align	2
	.p2align 4,,11
	.global	_cgo_allocate
	.type	_cgo_allocate, %function
_cgo_allocate:
.LVL4:
.LFB5:
	.loc 1 7 84 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 7 86 view .LVU15
	ret
	.cfi_endproc
.LFE5:
	.size	_cgo_allocate, .-_cgo_allocate
	.align	2
	.p2align 4,,11
	.global	_cgo_panic
	.type	_cgo_panic, %function
_cgo_panic:
.LFB9:
	.cfi_startproc
	ret
	.cfi_endproc
.LFE9:
	.size	_cgo_panic, .-_cgo_panic
	.align	2
	.p2align 4,,11
	.global	_cgo_reginit
	.type	_cgo_reginit, %function
_cgo_reginit:
.LFB7:
	.loc 1 9 25 view -0
	.cfi_startproc
	.loc 1 9 27 view .LVU17
	ret
	.cfi_endproc
.LFE7:
	.size	_cgo_reginit, .-_cgo_reginit
.Letext0:
	.file 2 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x1f0
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x9
	.4byte	.LASF14
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.4byte	.LLRL1
	.8byte	0
	.4byte	.Ldebug_line0
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF2
	.uleb128 0xa
	.4byte	.LASF15
	.byte	0x2
	.byte	0xd1
	.byte	0x17
	.4byte	0x3d
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
	.uleb128 0xb
	.4byte	.LASF16
	.byte	0x1
	.byte	0x9
	.byte	0x6
	.8byte	.LFB7
	.8byte	.LFE7-.LFB7
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0xc
	.4byte	.LASF17
	.byte	0x1
	.byte	0x8
	.byte	0x6
	.4byte	0x92
	.uleb128 0x2
	.string	"a"
	.byte	0x8
	.byte	0x17
	.4byte	0x92
	.uleb128 0x2
	.string	"c"
	.byte	0x8
	.byte	0x36
	.4byte	0x94
	.byte	0
	.uleb128 0xd
	.byte	0x8
	.uleb128 0xe
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0xf
	.4byte	.LASF18
	.byte	0x1
	.byte	0x7
	.byte	0x6
	.byte	0x1
	.4byte	0xbb
	.uleb128 0x2
	.string	"a"
	.byte	0x7
	.byte	0x1a
	.4byte	0x92
	.uleb128 0x2
	.string	"c"
	.byte	0x7
	.byte	0x39
	.4byte	0x94
	.byte	0
	.uleb128 0x6
	.4byte	.LASF8
	.byte	0x6
	.byte	0x7
	.4byte	0xd8
	.8byte	.LFB4
	.8byte	.LFE4-.LFB4
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x3
	.4byte	0xdd
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF7
	.uleb128 0x7
	.4byte	.LASF10
	.byte	0x5
	.8byte	.LFB3
	.8byte	.LFE3-.LFB3
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x10e
	.uleb128 0x4
	.4byte	.LASF12
	.byte	0x5
	.byte	0x22
	.4byte	0x31
	.uleb128 0x1
	.byte	0x50
	.byte	0
	.uleb128 0x6
	.4byte	.LASF9
	.byte	0x4
	.byte	0x8
	.4byte	0x31
	.8byte	.LFB2
	.8byte	.LFE2-.LFB2
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x7
	.4byte	.LASF11
	.byte	0x3
	.8byte	.LFB1
	.8byte	.LFE1-.LFB1
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x174
	.uleb128 0x5
	.string	"fn"
	.byte	0x17
	.4byte	0x17f
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x5
	.string	"a"
	.byte	0x41
	.4byte	0x92
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x5
	.string	"c"
	.byte	0x60
	.4byte	0x94
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x4
	.4byte	.LASF12
	.byte	0x3
	.byte	0x82
	.4byte	0x31
	.uleb128 0x1
	.byte	0x53
	.byte	0
	.uleb128 0x10
	.4byte	0x17f
	.uleb128 0x11
	.4byte	0x92
	.byte	0
	.uleb128 0x3
	.4byte	0x174
	.uleb128 0x12
	.4byte	.LASF19
	.byte	0x1
	.byte	0x2
	.byte	0x5
	.4byte	0x94
	.8byte	.LFB0
	.8byte	.LFE0-.LFB0
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x1c8
	.uleb128 0x13
	.4byte	.LASF20
	.byte	0x1
	.byte	0x2
	.byte	0xe
	.4byte	0x94
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x4
	.4byte	.LASF13
	.byte	0x2
	.byte	0x33
	.4byte	0x1c8
	.uleb128 0x1
	.byte	0x51
	.byte	0
	.uleb128 0x3
	.4byte	0xd8
	.uleb128 0x14
	.4byte	0x9b
	.8byte	.LFB5
	.8byte	.LFE5-.LFB5
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x8
	.4byte	0xa8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x8
	.4byte	0xb1
	.uleb128 0x1
	.byte	0x51
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
	.uleb128 0x5
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0x8
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 3
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x6
	.uleb128 0x2e
	.byte	0
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
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
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 6
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
	.uleb128 0x8
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x18
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
	.uleb128 0x55
	.uleb128 0x17
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x10
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0xa
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
	.uleb128 0xb
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
	.uleb128 0x1
	.uleb128 0x13
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
	.uleb128 0xf
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
	.uleb128 0x20
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x11
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x12
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
	.uleb128 0x13
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
	.uleb128 0x14
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x31
	.uleb128 0x13
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
.LVUS0:
	.uleb128 0
	.uleb128 .LVU3
	.uleb128 .LVU3
	.uleb128 0
.LLST0:
	.byte	0x6
	.8byte	.LVL0
	.byte	0x4
	.uleb128 .LVL0-.LVL0
	.uleb128 .LVL1-.LVL0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.LVL0
	.uleb128 .LFE0-.LVL0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.Ldebug_loc3:
	.section	.debug_aranges,"",@progbits
	.4byte	0x3c
	.2byte	0x2
	.4byte	.Ldebug_info0
	.byte	0x8
	.byte	0
	.2byte	0
	.2byte	0
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.8byte	.LFB0
	.8byte	.LFE0-.LFB0
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
	.byte	0x7
	.8byte	.Ltext0
	.uleb128 .Letext0-.Ltext0
	.byte	0x7
	.8byte	.LFB0
	.uleb128 .LFE0-.LFB0
	.byte	0
.Ldebug_ranges3:
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF5:
	.string	"long long int"
.LASF15:
	.string	"size_t"
.LASF10:
	.string	"_cgo_release_context"
.LASF16:
	.string	"_cgo_reginit"
.LASF14:
	.string	"GNU C17 11.4.0"
.LASF3:
	.string	"long unsigned int"
.LASF9:
	.string	"_cgo_wait_runtime_init_done"
.LASF19:
	.string	"main"
.LASF7:
	.string	"char"
.LASF17:
	.string	"_cgo_panic"
.LASF2:
	.string	"long int"
.LASF8:
	.string	"_cgo_topofstack"
.LASF20:
	.string	"argc"
.LASF11:
	.string	"crosscall2"
.LASF13:
	.string	"argv"
.LASF6:
	.string	"long double"
.LASF12:
	.string	"ctxt"
.LASF4:
	.string	"unsigned int"
.LASF18:
	.string	"_cgo_allocate"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/tmp/go-build"
.LASF0:
	.string	"_cgo_main.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
