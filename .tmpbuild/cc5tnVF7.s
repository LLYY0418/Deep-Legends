	.arch armv8-a
	.file	"cgo_unix_cgo.cgo2.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT-build" "cgo_unix_cgo.cgo2.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_C2func_getaddrinfo
	.type	_cgo_77133bf98b3a_C2func_getaddrinfo, %function
_cgo_77133bf98b3a_C2func_getaddrinfo:
.LVL0:
.LFB47:
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
	.loc 1 56 2 is_stmt 1 view .LVU4
	.loc 1 46 1 is_stmt 0 view .LVU5
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -16
	.cfi_offset 22, -8
	.loc 1 56 22 view .LVU6
	bl	_cgo_topofstack
.LVL2:
	.loc 1 56 22 view .LVU7
	mov	x22, x0
.LVL3:
	.loc 1 57 2 is_stmt 1 view .LVU8
	.loc 1 58 21 view .LVU9
	.loc 1 59 2 view .LVU10
	bl	__errno_location
.LVL4:
	.loc 1 59 2 is_stmt 0 view .LVU11
	mov	x20, x0
	.loc 1 60 11 view .LVU12
	ldp	x0, x1, [x19]
	ldp	x2, x3, [x19, 16]
	.loc 1 59 8 view .LVU13
	str	wzr, [x20]
	.loc 1 60 2 is_stmt 1 view .LVU14
	.loc 1 60 11 is_stmt 0 view .LVU15
	bl	getaddrinfo
.LVL5:
	.loc 1 61 13 view .LVU16
	ldr	w20, [x20]
	.loc 1 60 11 view .LVU17
	mov	w21, w0
.LVL6:
	.loc 1 61 2 is_stmt 1 view .LVU18
	.loc 1 62 21 view .LVU19
	.loc 1 63 2 view .LVU20
	.loc 1 63 36 is_stmt 0 view .LVU21
	bl	_cgo_topofstack
.LVL7:
	.loc 1 64 2 is_stmt 1 view .LVU22
	.loc 1 63 54 is_stmt 0 view .LVU23
	sub	x1, x0, x22
	.loc 1 67 1 view .LVU24
	mov	w0, w20
.LVL8:
	.loc 1 64 12 view .LVU25
	add	x19, x19, x1
.LVL9:
	.loc 1 64 12 view .LVU26
	str	w21, [x19, 32]
	.loc 1 65 48 is_stmt 1 view .LVU27
	.loc 1 66 2 view .LVU28
	.loc 1 67 1 is_stmt 0 view .LVU29
	ldp	x19, x20, [sp, 16]
.LVL10:
	.loc 1 67 1 view .LVU30
	ldp	x21, x22, [sp, 32]
.LVL11:
	.loc 1 67 1 view .LVU31
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
.LFE47:
	.size	_cgo_77133bf98b3a_C2func_getaddrinfo, .-_cgo_77133bf98b3a_C2func_getaddrinfo
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_free
	.type	_cgo_77133bf98b3a_Cfunc_free, %function
_cgo_77133bf98b3a_Cfunc_free:
.LVL12:
.LFB48:
	.loc 1 72 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 73 2 view .LVU33
	.loc 1 76 21 view .LVU34
	.loc 1 77 2 view .LVU35
	ldr	x0, [x0]
.LVL13:
	.loc 1 77 2 is_stmt 0 view .LVU36
	b	free
.LVL14:
	.cfi_endproc
.LFE48:
	.size	_cgo_77133bf98b3a_Cfunc_free, .-_cgo_77133bf98b3a_Cfunc_free
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_freeaddrinfo
	.type	_cgo_77133bf98b3a_Cfunc_freeaddrinfo, %function
_cgo_77133bf98b3a_Cfunc_freeaddrinfo:
.LVL15:
.LFB49:
	.loc 1 84 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 85 2 view .LVU38
	.loc 1 88 21 view .LVU39
	.loc 1 89 2 view .LVU40
	ldr	x0, [x0]
.LVL16:
	.loc 1 89 2 is_stmt 0 view .LVU41
	b	freeaddrinfo
.LVL17:
	.cfi_endproc
.LFE49:
	.size	_cgo_77133bf98b3a_Cfunc_freeaddrinfo, .-_cgo_77133bf98b3a_Cfunc_freeaddrinfo
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_gai_strerror
	.type	_cgo_77133bf98b3a_Cfunc_gai_strerror, %function
_cgo_77133bf98b3a_Cfunc_gai_strerror:
.LVL18:
.LFB50:
	.loc 1 96 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 97 2 view .LVU43
	.loc 1 96 1 is_stmt 0 view .LVU44
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	mov	x19, x0
.LVL19:
	.loc 1 102 2 is_stmt 1 view .LVU45
	.loc 1 96 1 is_stmt 0 view .LVU46
	str	x21, [sp, 32]
	.cfi_offset 21, -16
	.loc 1 102 22 view .LVU47
	bl	_cgo_topofstack
.LVL20:
	.loc 1 102 22 view .LVU48
	mov	x21, x0
.LVL21:
	.loc 1 103 2 is_stmt 1 view .LVU49
	.loc 1 104 21 view .LVU50
	.loc 1 105 2 view .LVU51
	.loc 1 105 11 is_stmt 0 view .LVU52
	ldr	w0, [x19]
.LVL22:
	.loc 1 105 11 view .LVU53
	bl	gai_strerror
.LVL23:
	mov	x20, x0
.LVL24:
	.loc 1 106 21 is_stmt 1 view .LVU54
	.loc 1 107 2 view .LVU55
	.loc 1 107 36 is_stmt 0 view .LVU56
	bl	_cgo_topofstack
.LVL25:
	.loc 1 108 2 is_stmt 1 view .LVU57
	.loc 1 107 54 is_stmt 0 view .LVU58
	sub	x0, x0, x21
.LVL26:
	.loc 1 108 12 view .LVU59
	add	x19, x19, x0
.LVL27:
	.loc 1 110 1 view .LVU60
	ldr	x21, [sp, 32]
.LVL28:
	.loc 1 108 12 view .LVU61
	str	x20, [x19, 8]
	.loc 1 109 48 is_stmt 1 view .LVU62
	.loc 1 110 1 is_stmt 0 view .LVU63
	ldp	x19, x20, [sp, 16]
.LVL29:
	.loc 1 110 1 view .LVU64
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE50:
	.size	_cgo_77133bf98b3a_Cfunc_gai_strerror, .-_cgo_77133bf98b3a_Cfunc_gai_strerror
	.align	2
	.p2align 4,,11
	.global	_cgo_77133bf98b3a_Cfunc_getaddrinfo
	.type	_cgo_77133bf98b3a_Cfunc_getaddrinfo, %function
_cgo_77133bf98b3a_Cfunc_getaddrinfo:
.LVL30:
.LFB51:
	.loc 1 115 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 116 2 view .LVU66
	.loc 1 115 1 is_stmt 0 view .LVU67
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	mov	x19, x0
.LVL31:
	.loc 1 124 2 is_stmt 1 view .LVU68
	.loc 1 115 1 is_stmt 0 view .LVU69
	str	x21, [sp, 32]
	.cfi_offset 21, -16
	.loc 1 124 22 view .LVU70
	bl	_cgo_topofstack
.LVL32:
	.loc 1 124 22 view .LVU71
	mov	x21, x0
.LVL33:
	.loc 1 125 2 is_stmt 1 view .LVU72
	.loc 1 126 21 view .LVU73
	.loc 1 127 2 view .LVU74
	.loc 1 127 11 is_stmt 0 view .LVU75
	ldp	x0, x1, [x19]
.LVL34:
	.loc 1 127 11 view .LVU76
	ldp	x2, x3, [x19, 16]
	bl	getaddrinfo
.LVL35:
	mov	w20, w0
.LVL36:
	.loc 1 128 21 is_stmt 1 view .LVU77
	.loc 1 129 2 view .LVU78
	.loc 1 129 36 is_stmt 0 view .LVU79
	bl	_cgo_topofstack
.LVL37:
	.loc 1 130 2 is_stmt 1 view .LVU80
	.loc 1 129 54 is_stmt 0 view .LVU81
	sub	x0, x0, x21
.LVL38:
	.loc 1 130 12 view .LVU82
	add	x19, x19, x0
.LVL39:
	.loc 1 132 1 view .LVU83
	ldr	x21, [sp, 32]
.LVL40:
	.loc 1 130 12 view .LVU84
	str	w20, [x19, 32]
	.loc 1 131 48 is_stmt 1 view .LVU85
	.loc 1 132 1 is_stmt 0 view .LVU86
	ldp	x19, x20, [sp, 16]
.LVL41:
	.loc 1 132 1 view .LVU87
	ldp	x29, x30, [sp], 48
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE51:
	.size	_cgo_77133bf98b3a_Cfunc_getaddrinfo, .-_cgo_77133bf98b3a_Cfunc_getaddrinfo
.Letext0:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 3 "/usr/include/aarch64-linux-gnu/bits/socket.h"
	.file 4 "/usr/include/aarch64-linux-gnu/bits/sockaddr.h"
	.file 5 "/usr/include/netinet/in.h"
	.file 6 "/usr/include/aarch64-linux-gnu/bits/stdint-uintn.h"
	.file 7 "/usr/include/netdb.h"
	.file 8 "/usr/include/stdlib.h"
	.file 9 "/usr/include/errno.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x88e
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x19
	.4byte	.LASF80
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x7
	.byte	0x8
	.byte	0x5
	.4byte	.LASF2
	.uleb128 0x7
	.byte	0x8
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x7
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x7
	.byte	0x8
	.byte	0x5
	.4byte	.LASF5
	.uleb128 0x7
	.byte	0x10
	.byte	0x4
	.4byte	.LASF6
	.uleb128 0x1
	.4byte	0x62
	.uleb128 0x2
	.4byte	0x51
	.uleb128 0x7
	.byte	0x1
	.byte	0x8
	.4byte	.LASF7
	.uleb128 0x3
	.4byte	0x5b
	.uleb128 0x1
	.4byte	0x5b
	.uleb128 0x7
	.byte	0x1
	.byte	0x8
	.4byte	.LASF8
	.uleb128 0x7
	.byte	0x2
	.byte	0x7
	.4byte	.LASF9
	.uleb128 0x7
	.byte	0x1
	.byte	0x6
	.4byte	.LASF10
	.uleb128 0x8
	.4byte	.LASF12
	.byte	0x2
	.byte	0x26
	.byte	0x17
	.4byte	0x6c
	.uleb128 0x7
	.byte	0x2
	.byte	0x5
	.4byte	.LASF11
	.uleb128 0x8
	.4byte	.LASF13
	.byte	0x2
	.byte	0x28
	.byte	0x1c
	.4byte	0x73
	.uleb128 0x1a
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x8
	.4byte	.LASF14
	.byte	0x2
	.byte	0x2a
	.byte	0x16
	.4byte	0x3c
	.uleb128 0x1b
	.byte	0x8
	.uleb128 0x8
	.4byte	.LASF15
	.byte	0x2
	.byte	0xd2
	.byte	0x17
	.4byte	0x3c
	.uleb128 0x7
	.byte	0x8
	.byte	0x7
	.4byte	.LASF16
	.uleb128 0x8
	.4byte	.LASF17
	.byte	0x3
	.byte	0x21
	.byte	0x15
	.4byte	0xb5
	.uleb128 0x8
	.4byte	.LASF18
	.byte	0x4
	.byte	0x1c
	.byte	0x1c
	.4byte	0x73
	.uleb128 0x11
	.4byte	.LASF25
	.byte	0x10
	.byte	0x3
	.byte	0xb4
	.4byte	0x107
	.uleb128 0x9
	.4byte	.LASF19
	.byte	0x3
	.byte	0xb6
	.byte	0x5
	.4byte	0xd4
	.byte	0
	.uleb128 0x9
	.4byte	.LASF20
	.byte	0x3
	.byte	0xb7
	.byte	0xa
	.4byte	0x10c
	.byte	0x2
	.byte	0
	.uleb128 0x3
	.4byte	0xe0
	.uleb128 0xd
	.4byte	0x5b
	.4byte	0x11c
	.uleb128 0xe
	.4byte	0x35
	.byte	0xd
	.byte	0
	.uleb128 0x1
	.4byte	0xe0
	.uleb128 0x2
	.4byte	0x11c
	.uleb128 0xa
	.4byte	.LASF21
	.uleb128 0x3
	.4byte	0x126
	.uleb128 0x1
	.4byte	0x126
	.uleb128 0x2
	.4byte	0x130
	.uleb128 0xa
	.4byte	.LASF22
	.uleb128 0x3
	.4byte	0x13a
	.uleb128 0x1
	.4byte	0x13a
	.uleb128 0x2
	.4byte	0x144
	.uleb128 0xa
	.4byte	.LASF23
	.uleb128 0x3
	.4byte	0x14e
	.uleb128 0x1
	.4byte	0x14e
	.uleb128 0x2
	.4byte	0x158
	.uleb128 0xa
	.4byte	.LASF24
	.uleb128 0x3
	.4byte	0x162
	.uleb128 0x1
	.4byte	0x162
	.uleb128 0x2
	.4byte	0x16c
	.uleb128 0x11
	.4byte	.LASF26
	.byte	0x10
	.byte	0x5
	.byte	0xf5
	.4byte	0x1b7
	.uleb128 0x9
	.4byte	.LASF27
	.byte	0x5
	.byte	0xf7
	.byte	0x5
	.4byte	0xd4
	.byte	0
	.uleb128 0x9
	.4byte	.LASF28
	.byte	0x5
	.byte	0xf8
	.byte	0xf
	.4byte	0x36d
	.byte	0x2
	.uleb128 0x9
	.4byte	.LASF29
	.byte	0x5
	.byte	0xf9
	.byte	0x14
	.4byte	0x353
	.byte	0x4
	.uleb128 0x9
	.4byte	.LASF30
	.byte	0x5
	.byte	0xfc
	.byte	0x13
	.4byte	0x3ee
	.byte	0x8
	.byte	0
	.uleb128 0x3
	.4byte	0x176
	.uleb128 0x1
	.4byte	0x176
	.uleb128 0x2
	.4byte	0x1bc
	.uleb128 0x14
	.4byte	.LASF31
	.byte	0x1c
	.byte	0x5
	.2byte	0x104
	.4byte	0x21a
	.uleb128 0x5
	.4byte	.LASF32
	.byte	0x5
	.2byte	0x106
	.byte	0x5
	.4byte	0xd4
	.byte	0
	.uleb128 0x5
	.4byte	.LASF33
	.byte	0x5
	.2byte	0x107
	.byte	0xf
	.4byte	0x36d
	.byte	0x2
	.uleb128 0x5
	.4byte	.LASF34
	.byte	0x5
	.2byte	0x108
	.byte	0xe
	.4byte	0x33b
	.byte	0x4
	.uleb128 0x5
	.4byte	.LASF35
	.byte	0x5
	.2byte	0x109
	.byte	0x15
	.4byte	0x3d4
	.byte	0x8
	.uleb128 0x5
	.4byte	.LASF36
	.byte	0x5
	.2byte	0x10a
	.byte	0xe
	.4byte	0x33b
	.byte	0x18
	.byte	0
	.uleb128 0x3
	.4byte	0x1c6
	.uleb128 0x1
	.4byte	0x1c6
	.uleb128 0x2
	.4byte	0x21f
	.uleb128 0xa
	.4byte	.LASF37
	.uleb128 0x3
	.4byte	0x229
	.uleb128 0x1
	.4byte	0x229
	.uleb128 0x2
	.4byte	0x233
	.uleb128 0xa
	.4byte	.LASF38
	.uleb128 0x3
	.4byte	0x23d
	.uleb128 0x1
	.4byte	0x23d
	.uleb128 0x2
	.4byte	0x247
	.uleb128 0xa
	.4byte	.LASF39
	.uleb128 0x3
	.4byte	0x251
	.uleb128 0x1
	.4byte	0x251
	.uleb128 0x2
	.4byte	0x25b
	.uleb128 0xa
	.4byte	.LASF40
	.uleb128 0x3
	.4byte	0x265
	.uleb128 0x1
	.4byte	0x265
	.uleb128 0x2
	.4byte	0x26f
	.uleb128 0xa
	.4byte	.LASF41
	.uleb128 0x3
	.4byte	0x279
	.uleb128 0x1
	.4byte	0x279
	.uleb128 0x2
	.4byte	0x283
	.uleb128 0xa
	.4byte	.LASF42
	.uleb128 0x3
	.4byte	0x28d
	.uleb128 0x1
	.4byte	0x28d
	.uleb128 0x2
	.4byte	0x297
	.uleb128 0x1
	.4byte	0x107
	.uleb128 0x2
	.4byte	0x2a1
	.uleb128 0x1
	.4byte	0x12b
	.uleb128 0x2
	.4byte	0x2ab
	.uleb128 0x1
	.4byte	0x13f
	.uleb128 0x2
	.4byte	0x2b5
	.uleb128 0x1
	.4byte	0x153
	.uleb128 0x2
	.4byte	0x2bf
	.uleb128 0x1
	.4byte	0x167
	.uleb128 0x2
	.4byte	0x2c9
	.uleb128 0x1
	.4byte	0x1b7
	.uleb128 0x2
	.4byte	0x2d3
	.uleb128 0x1
	.4byte	0x21a
	.uleb128 0x2
	.4byte	0x2dd
	.uleb128 0x1
	.4byte	0x22e
	.uleb128 0x2
	.4byte	0x2e7
	.uleb128 0x1
	.4byte	0x242
	.uleb128 0x2
	.4byte	0x2f1
	.uleb128 0x1
	.4byte	0x256
	.uleb128 0x2
	.4byte	0x2fb
	.uleb128 0x1
	.4byte	0x26a
	.uleb128 0x2
	.4byte	0x305
	.uleb128 0x1
	.4byte	0x27e
	.uleb128 0x2
	.4byte	0x30f
	.uleb128 0x1
	.4byte	0x292
	.uleb128 0x2
	.4byte	0x319
	.uleb128 0x8
	.4byte	.LASF43
	.byte	0x6
	.byte	0x18
	.byte	0x13
	.4byte	0x81
	.uleb128 0x8
	.4byte	.LASF44
	.byte	0x6
	.byte	0x19
	.byte	0x14
	.4byte	0x94
	.uleb128 0x8
	.4byte	.LASF45
	.byte	0x6
	.byte	0x1a
	.byte	0x14
	.4byte	0xa7
	.uleb128 0x8
	.4byte	.LASF46
	.byte	0x5
	.byte	0x1e
	.byte	0x12
	.4byte	0x33b
	.uleb128 0x11
	.4byte	.LASF47
	.byte	0x4
	.byte	0x5
	.byte	0x1f
	.4byte	0x36d
	.uleb128 0x9
	.4byte	.LASF48
	.byte	0x5
	.byte	0x21
	.byte	0xf
	.4byte	0x347
	.byte	0
	.byte	0
	.uleb128 0x8
	.4byte	.LASF49
	.byte	0x5
	.byte	0x7b
	.byte	0x12
	.4byte	0x32f
	.uleb128 0x1c
	.byte	0x10
	.byte	0x5
	.byte	0xdd
	.byte	0x5
	.4byte	0x3a4
	.uleb128 0x13
	.4byte	.LASF50
	.byte	0xdf
	.byte	0xa
	.4byte	0x3a4
	.uleb128 0x13
	.4byte	.LASF51
	.byte	0xe0
	.byte	0xb
	.4byte	0x3b4
	.uleb128 0x13
	.4byte	.LASF52
	.byte	0xe1
	.byte	0xb
	.4byte	0x3c4
	.byte	0
	.uleb128 0xd
	.4byte	0x323
	.4byte	0x3b4
	.uleb128 0xe
	.4byte	0x35
	.byte	0xf
	.byte	0
	.uleb128 0xd
	.4byte	0x32f
	.4byte	0x3c4
	.uleb128 0xe
	.4byte	0x35
	.byte	0x7
	.byte	0
	.uleb128 0xd
	.4byte	0x33b
	.4byte	0x3d4
	.uleb128 0xe
	.4byte	0x35
	.byte	0x3
	.byte	0
	.uleb128 0x11
	.4byte	.LASF53
	.byte	0x10
	.byte	0x5
	.byte	0xdb
	.4byte	0x3ee
	.uleb128 0x9
	.4byte	.LASF54
	.byte	0x5
	.byte	0xe2
	.byte	0x9
	.4byte	0x379
	.byte	0
	.byte	0
	.uleb128 0xd
	.4byte	0x6c
	.4byte	0x3fe
	.uleb128 0xe
	.4byte	0x35
	.byte	0x7
	.byte	0
	.uleb128 0x14
	.4byte	.LASF55
	.byte	0x30
	.byte	0x7
	.2byte	0x235
	.4byte	0x47c
	.uleb128 0x5
	.4byte	.LASF56
	.byte	0x7
	.2byte	0x237
	.byte	0x7
	.4byte	0xa0
	.byte	0
	.uleb128 0x5
	.4byte	.LASF57
	.byte	0x7
	.2byte	0x238
	.byte	0x7
	.4byte	0xa0
	.byte	0x4
	.uleb128 0x5
	.4byte	.LASF58
	.byte	0x7
	.2byte	0x239
	.byte	0x7
	.4byte	0xa0
	.byte	0x8
	.uleb128 0x5
	.4byte	.LASF59
	.byte	0x7
	.2byte	0x23a
	.byte	0x7
	.4byte	0xa0
	.byte	0xc
	.uleb128 0x5
	.4byte	.LASF60
	.byte	0x7
	.2byte	0x23b
	.byte	0xd
	.4byte	0xc8
	.byte	0x10
	.uleb128 0x5
	.4byte	.LASF61
	.byte	0x7
	.2byte	0x23c
	.byte	0x14
	.4byte	0x11c
	.byte	0x18
	.uleb128 0x5
	.4byte	.LASF62
	.byte	0x7
	.2byte	0x23d
	.byte	0x9
	.4byte	0x67
	.byte	0x20
	.uleb128 0x5
	.4byte	.LASF63
	.byte	0x7
	.2byte	0x23e
	.byte	0x14
	.4byte	0x481
	.byte	0x28
	.byte	0
	.uleb128 0x3
	.4byte	0x3fe
	.uleb128 0x1
	.4byte	0x3fe
	.uleb128 0x1
	.4byte	0x47c
	.uleb128 0x2
	.4byte	0x486
	.uleb128 0x15
	.4byte	.LASF66
	.2byte	0x29d
	.byte	0x14
	.4byte	0x51
	.4byte	0x4a6
	.uleb128 0xc
	.4byte	0xa0
	.byte	0
	.uleb128 0x16
	.4byte	.LASF64
	.byte	0x7
	.2byte	0x29a
	.4byte	0x4b8
	.uleb128 0xc
	.4byte	0x481
	.byte	0
	.uleb128 0x16
	.4byte	.LASF65
	.byte	0x8
	.2byte	0x22b
	.4byte	0x4ca
	.uleb128 0xc
	.4byte	0xb3
	.byte	0
	.uleb128 0x15
	.4byte	.LASF67
	.2byte	0x294
	.byte	0xc
	.4byte	0xa0
	.4byte	0x4ef
	.uleb128 0xc
	.4byte	0x56
	.uleb128 0xc
	.4byte	0x56
	.uleb128 0xc
	.4byte	0x48b
	.uleb128 0xc
	.4byte	0x4f4
	.byte	0
	.uleb128 0x1
	.4byte	0x481
	.uleb128 0x2
	.4byte	0x4ef
	.uleb128 0x17
	.4byte	.LASF68
	.byte	0x9
	.byte	0x25
	.byte	0xd
	.4byte	0x505
	.uleb128 0x1
	.4byte	0xa0
	.uleb128 0x17
	.4byte	.LASF69
	.byte	0x1
	.byte	0x13
	.byte	0xe
	.4byte	0x67
	.uleb128 0x12
	.4byte	.LASF74
	.byte	0x72
	.8byte	.LFB51
	.8byte	.LFE51-.LFB51
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x5ef
	.uleb128 0xf
	.string	"v"
	.byte	0x72
	.byte	0x2b
	.4byte	0xb3
	.4byte	.LLST13
	.4byte	.LVUS13
	.uleb128 0x10
	.byte	0x28
	.byte	0x74
	.4byte	0x58e
	.uleb128 0x4
	.string	"p0"
	.byte	0x75
	.byte	0xf
	.4byte	0x51
	.byte	0
	.uleb128 0x4
	.string	"p1"
	.byte	0x76
	.byte	0xf
	.4byte	0x51
	.byte	0x8
	.uleb128 0x4
	.string	"p2"
	.byte	0x77
	.byte	0x1a
	.4byte	0x486
	.byte	0x10
	.uleb128 0x4
	.string	"p3"
	.byte	0x78
	.byte	0x15
	.4byte	0x4ef
	.byte	0x18
	.uleb128 0x4
	.string	"r"
	.byte	0x79
	.byte	0x7
	.4byte	0xa0
	.byte	0x20
	.uleb128 0x9
	.4byte	.LASF70
	.byte	0x1
	.byte	0x7a
	.byte	0x8
	.4byte	0x5ef
	.byte	0x24
	.byte	0
	.uleb128 0x6
	.4byte	.LASF71
	.byte	0x7b
	.byte	0x21
	.4byte	0x5ff
	.4byte	.LLST14
	.4byte	.LVUS14
	.uleb128 0x6
	.4byte	.LASF72
	.byte	0x7c
	.byte	0x8
	.4byte	0x67
	.4byte	.LLST15
	.4byte	.LVUS15
	.uleb128 0x6
	.4byte	.LASF73
	.byte	0x7d
	.byte	0x18
	.4byte	0xa0
	.4byte	.LLST16
	.4byte	.LVUS16
	.uleb128 0xb
	.8byte	.LVL32
	.4byte	0x50a
	.uleb128 0xb
	.8byte	.LVL35
	.4byte	0x4ca
	.uleb128 0xb
	.8byte	.LVL37
	.4byte	0x50a
	.byte	0
	.uleb128 0xd
	.4byte	0x5b
	.4byte	0x5ff
	.uleb128 0xe
	.4byte	0x35
	.byte	0x3
	.byte	0
	.uleb128 0x1
	.4byte	0x543
	.uleb128 0x12
	.4byte	.LASF75
	.byte	0x5f
	.8byte	.LFB50
	.8byte	.LFE50-.LFB50
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x6bc
	.uleb128 0xf
	.string	"v"
	.byte	0x5f
	.byte	0x2c
	.4byte	0xb3
	.4byte	.LLST9
	.4byte	.LVUS9
	.uleb128 0x10
	.byte	0x10
	.byte	0x61
	.4byte	0x65b
	.uleb128 0x4
	.string	"p0"
	.byte	0x62
	.byte	0x7
	.4byte	0xa0
	.byte	0
	.uleb128 0x9
	.4byte	.LASF76
	.byte	0x1
	.byte	0x63
	.byte	0x8
	.4byte	0x5ef
	.byte	0x4
	.uleb128 0x4
	.string	"r"
	.byte	0x64
	.byte	0xf
	.4byte	0x51
	.byte	0x8
	.byte	0
	.uleb128 0x6
	.4byte	.LASF71
	.byte	0x65
	.byte	0x21
	.4byte	0x6bc
	.4byte	.LLST10
	.4byte	.LVUS10
	.uleb128 0x6
	.4byte	.LASF72
	.byte	0x66
	.byte	0x8
	.4byte	0x67
	.4byte	.LLST11
	.4byte	.LVUS11
	.uleb128 0x6
	.4byte	.LASF73
	.byte	0x67
	.byte	0x18
	.4byte	0x51
	.4byte	.LLST12
	.4byte	.LVUS12
	.uleb128 0xb
	.8byte	.LVL20
	.4byte	0x50a
	.uleb128 0xb
	.8byte	.LVL23
	.4byte	0x490
	.uleb128 0xb
	.8byte	.LVL25
	.4byte	0x50a
	.byte	0
	.uleb128 0x1
	.4byte	0x631
	.uleb128 0x12
	.4byte	.LASF77
	.byte	0x53
	.8byte	.LFB49
	.8byte	.LFE49-.LFB49
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x722
	.uleb128 0xf
	.string	"v"
	.byte	0x53
	.byte	0x2c
	.4byte	0xb3
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x10
	.byte	0x8
	.byte	0x55
	.4byte	0x701
	.uleb128 0x4
	.string	"p0"
	.byte	0x56
	.byte	0x14
	.4byte	0x481
	.byte	0
	.byte	0
	.uleb128 0x6
	.4byte	.LASF71
	.byte	0x57
	.byte	0x21
	.4byte	0x722
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x18
	.8byte	.LVL17
	.4byte	0x4a6
	.byte	0
	.uleb128 0x1
	.4byte	0x6ee
	.uleb128 0x12
	.4byte	.LASF78
	.byte	0x47
	.8byte	.LFB48
	.8byte	.LFE48-.LFB48
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x788
	.uleb128 0xf
	.string	"v"
	.byte	0x47
	.byte	0x24
	.4byte	0xb3
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0x10
	.byte	0x8
	.byte	0x49
	.4byte	0x767
	.uleb128 0x4
	.string	"p0"
	.byte	0x4a
	.byte	0x9
	.4byte	0xb3
	.byte	0
	.byte	0
	.uleb128 0x6
	.4byte	.LASF71
	.byte	0x4b
	.byte	0x21
	.4byte	0x788
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x18
	.8byte	.LVL14
	.4byte	0x4b8
	.byte	0
	.uleb128 0x1
	.4byte	0x754
	.uleb128 0x1d
	.4byte	.LASF81
	.byte	0x1
	.byte	0x2d
	.byte	0x1
	.4byte	0xa0
	.8byte	.LFB47
	.8byte	.LFE47-.LFB47
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x88c
	.uleb128 0xf
	.string	"v"
	.byte	0x2d
	.byte	0x2c
	.4byte	0xb3
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x6
	.4byte	.LASF79
	.byte	0x2f
	.byte	0x6
	.4byte	0xa0
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x10
	.byte	0x28
	.byte	0x30
	.4byte	0x81e
	.uleb128 0x4
	.string	"p0"
	.byte	0x31
	.byte	0xf
	.4byte	0x51
	.byte	0
	.uleb128 0x4
	.string	"p1"
	.byte	0x32
	.byte	0xf
	.4byte	0x51
	.byte	0x8
	.uleb128 0x4
	.string	"p2"
	.byte	0x33
	.byte	0x1a
	.4byte	0x486
	.byte	0x10
	.uleb128 0x4
	.string	"p3"
	.byte	0x34
	.byte	0x15
	.4byte	0x4ef
	.byte	0x18
	.uleb128 0x4
	.string	"r"
	.byte	0x35
	.byte	0x7
	.4byte	0xa0
	.byte	0x20
	.uleb128 0x9
	.4byte	.LASF70
	.byte	0x1
	.byte	0x36
	.byte	0x8
	.4byte	0x5ef
	.byte	0x24
	.byte	0
	.uleb128 0x6
	.4byte	.LASF71
	.byte	0x37
	.byte	0x21
	.4byte	0x88c
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x6
	.4byte	.LASF72
	.byte	0x38
	.byte	0x8
	.4byte	0x67
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x6
	.4byte	.LASF73
	.byte	0x39
	.byte	0x18
	.4byte	0xa0
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0xb
	.8byte	.LVL2
	.4byte	0x50a
	.uleb128 0xb
	.8byte	.LVL4
	.4byte	0x4f9
	.uleb128 0xb
	.8byte	.LVL5
	.4byte	0x4ca
	.uleb128 0xb
	.8byte	.LVL7
	.4byte	0x50a
	.byte	0
	.uleb128 0x1
	.4byte	0x7d3
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x3
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x4
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
	.uleb128 0x5
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0x5
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x38
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x6
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
	.uleb128 0x7
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
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xc
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
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
	.uleb128 0x10
	.uleb128 0x13
	.byte	0x1
	.uleb128 0xb
	.uleb128 0xb
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
	.uleb128 0x11
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
	.uleb128 0x12
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
	.sleb128 1
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
	.uleb128 0x13
	.uleb128 0xd
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 5
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x14
	.uleb128 0x13
	.byte	0x1
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0x5
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 8
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
	.uleb128 0x21
	.sleb128 7
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
	.uleb128 0x5
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 13
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x17
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
	.uleb128 0x18
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
	.uleb128 0x19
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
	.uleb128 0x1a
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
	.uleb128 0x1b
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x1c
	.uleb128 0x17
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
.LVUS13:
	.uleb128 0
	.uleb128 .LVU71
	.uleb128 .LVU71
	.uleb128 .LVU83
	.uleb128 .LVU83
	.uleb128 0
.LLST13:
	.byte	0x4
	.uleb128 .LVL30-.Ltext0
	.uleb128 .LVL32-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL32-1-.Ltext0
	.uleb128 .LVL39-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL39-.Ltext0
	.uleb128 .LFE51-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS14:
	.uleb128 .LVU68
	.uleb128 .LVU71
	.uleb128 .LVU71
	.uleb128 .LVU80
	.uleb128 .LVU80
	.uleb128 .LVU82
.LLST14:
	.byte	0x4
	.uleb128 .LVL31-.Ltext0
	.uleb128 .LVL32-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL32-1-.Ltext0
	.uleb128 .LVL37-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL37-.Ltext0
	.uleb128 .LVL38-.Ltext0
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
.LVUS15:
	.uleb128 .LVU72
	.uleb128 .LVU76
	.uleb128 .LVU76
	.uleb128 .LVU84
.LLST15:
	.byte	0x4
	.uleb128 .LVL33-.Ltext0
	.uleb128 .LVL34-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL40-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS16:
	.uleb128 .LVU77
	.uleb128 .LVU80
	.uleb128 .LVU80
	.uleb128 .LVU87
	.uleb128 .LVU87
	.uleb128 0
.LLST16:
	.byte	0x4
	.uleb128 .LVL36-.Ltext0
	.uleb128 .LVL37-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL37-1-.Ltext0
	.uleb128 .LVL41-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL41-.Ltext0
	.uleb128 .LFE51-.Ltext0
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
.LVUS9:
	.uleb128 0
	.uleb128 .LVU48
	.uleb128 .LVU48
	.uleb128 .LVU60
	.uleb128 .LVU60
	.uleb128 0
.LLST9:
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LVL20-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL20-1-.Ltext0
	.uleb128 .LVL27-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL27-.Ltext0
	.uleb128 .LFE50-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS10:
	.uleb128 .LVU45
	.uleb128 .LVU48
	.uleb128 .LVU48
	.uleb128 .LVU57
	.uleb128 .LVU57
	.uleb128 .LVU59
.LLST10:
	.byte	0x4
	.uleb128 .LVL19-.Ltext0
	.uleb128 .LVL20-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL20-1-.Ltext0
	.uleb128 .LVL25-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL25-.Ltext0
	.uleb128 .LVL26-.Ltext0
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
.LVUS11:
	.uleb128 .LVU49
	.uleb128 .LVU53
	.uleb128 .LVU53
	.uleb128 .LVU61
.LLST11:
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LVL28-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS12:
	.uleb128 .LVU54
	.uleb128 .LVU57
	.uleb128 .LVU57
	.uleb128 .LVU64
	.uleb128 .LVU64
	.uleb128 0
.LLST12:
	.byte	0x4
	.uleb128 .LVL24-.Ltext0
	.uleb128 .LVL25-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL25-1-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LFE50-.Ltext0
	.uleb128 0x8
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x70
	.sleb128 0
	.byte	0x22
	.byte	0x23
	.uleb128 0x8
	.byte	0
.LVUS7:
	.uleb128 0
	.uleb128 .LVU41
	.uleb128 .LVU41
	.uleb128 0
.LLST7:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL16-.Ltext0
	.uleb128 .LFE49-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS8:
	.uleb128 .LVU39
	.uleb128 .LVU41
	.uleb128 .LVU41
	.uleb128 0
.LLST8:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL16-.Ltext0
	.uleb128 .LFE49-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS5:
	.uleb128 0
	.uleb128 .LVU36
	.uleb128 .LVU36
	.uleb128 0
.LLST5:
	.byte	0x4
	.uleb128 .LVL12-.Ltext0
	.uleb128 .LVL13-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL13-.Ltext0
	.uleb128 .LFE48-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS6:
	.uleb128 .LVU34
	.uleb128 .LVU36
	.uleb128 .LVU36
	.uleb128 0
.LLST6:
	.byte	0x4
	.uleb128 .LVL12-.Ltext0
	.uleb128 .LVL13-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL13-.Ltext0
	.uleb128 .LFE48-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
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
	.uleb128 .LFE47-.Ltext0
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
	.uleb128 .LFE47-.Ltext0
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
	.uleb128 .LFE47-.Ltext0
	.uleb128 0x8
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x71
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
.LASF68:
	.string	"__errno_location"
.LASF15:
	.string	"__socklen_t"
.LASF22:
	.string	"sockaddr_ax25"
.LASF73:
	.string	"_cgo_r"
.LASF60:
	.string	"ai_addrlen"
.LASF70:
	.string	"__pad36"
.LASF39:
	.string	"sockaddr_iso"
.LASF53:
	.string	"in6_addr"
.LASF74:
	.string	"_cgo_77133bf98b3a_Cfunc_getaddrinfo"
.LASF37:
	.string	"sockaddr_inarp"
.LASF5:
	.string	"long long int"
.LASF14:
	.string	"__uint32_t"
.LASF34:
	.string	"sin6_flowinfo"
.LASF35:
	.string	"sin6_addr"
.LASF13:
	.string	"__uint16_t"
.LASF75:
	.string	"_cgo_77133bf98b3a_Cfunc_gai_strerror"
.LASF11:
	.string	"short int"
.LASF55:
	.string	"addrinfo"
.LASF43:
	.string	"uint8_t"
.LASF36:
	.string	"sin6_scope_id"
.LASF27:
	.string	"sin_family"
.LASF65:
	.string	"free"
.LASF18:
	.string	"sa_family_t"
.LASF69:
	.string	"_cgo_topofstack"
.LASF33:
	.string	"sin6_port"
.LASF54:
	.string	"__in6_u"
.LASF58:
	.string	"ai_socktype"
.LASF40:
	.string	"sockaddr_ns"
.LASF38:
	.string	"sockaddr_ipx"
.LASF51:
	.string	"__u6_addr16"
.LASF2:
	.string	"long int"
.LASF61:
	.string	"ai_addr"
.LASF41:
	.string	"sockaddr_un"
.LASF12:
	.string	"__uint8_t"
.LASF29:
	.string	"sin_addr"
.LASF57:
	.string	"ai_family"
.LASF6:
	.string	"long double"
.LASF67:
	.string	"getaddrinfo"
.LASF32:
	.string	"sin6_family"
.LASF8:
	.string	"unsigned char"
.LASF17:
	.string	"socklen_t"
.LASF64:
	.string	"freeaddrinfo"
.LASF10:
	.string	"signed char"
.LASF30:
	.string	"sin_zero"
.LASF16:
	.string	"long long unsigned int"
.LASF31:
	.string	"sockaddr_in6"
.LASF45:
	.string	"uint32_t"
.LASF63:
	.string	"ai_next"
.LASF80:
	.string	"GNU C17 11.4.0"
.LASF66:
	.string	"gai_strerror"
.LASF48:
	.string	"s_addr"
.LASF81:
	.string	"_cgo_77133bf98b3a_C2func_getaddrinfo"
.LASF20:
	.string	"sa_data"
.LASF9:
	.string	"short unsigned int"
.LASF77:
	.string	"_cgo_77133bf98b3a_Cfunc_freeaddrinfo"
.LASF23:
	.string	"sockaddr_dl"
.LASF7:
	.string	"char"
.LASF44:
	.string	"uint16_t"
.LASF76:
	.string	"__pad4"
.LASF46:
	.string	"in_addr_t"
.LASF24:
	.string	"sockaddr_eon"
.LASF78:
	.string	"_cgo_77133bf98b3a_Cfunc_free"
.LASF3:
	.string	"long unsigned int"
.LASF47:
	.string	"in_addr"
.LASF42:
	.string	"sockaddr_x25"
.LASF62:
	.string	"ai_canonname"
.LASF28:
	.string	"sin_port"
.LASF56:
	.string	"ai_flags"
.LASF49:
	.string	"in_port_t"
.LASF19:
	.string	"sa_family"
.LASF21:
	.string	"sockaddr_at"
.LASF52:
	.string	"__u6_addr32"
.LASF72:
	.string	"_cgo_stktop"
.LASF4:
	.string	"unsigned int"
.LASF50:
	.string	"__u6_addr8"
.LASF26:
	.string	"sockaddr_in"
.LASF25:
	.string	"sockaddr"
.LASF71:
	.string	"_cgo_a"
.LASF79:
	.string	"_cgo_errno"
.LASF59:
	.string	"ai_protocol"
	.section	.debug_line_str,"MS",@progbits,1
.LASF0:
	.string	"cgo_unix_cgo.cgo2.c"
.LASF1:
	.string	"/tmp/go-build"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
