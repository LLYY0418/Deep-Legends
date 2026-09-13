	.arch armv8-a
	.file	"gcc_sigaction.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_sigaction.c"
	.align	2
	.p2align 4,,11
	.global	x_cgo_sigaction
	.type	x_cgo_sigaction, %function
x_cgo_sigaction:
.LVL0:
.LFB51:
	.file 1 "gcc_sigaction.c"
	.loc 1 32 89 view -0
	.cfi_startproc
	.loc 1 33 2 view .LVU1
	.loc 1 34 2 view .LVU2
	.loc 1 35 2 view .LVU3
	.loc 1 36 2 view .LVU4
	.loc 1 38 21 view .LVU5
	.loc 1 40 2 view .LVU6
.LBB6:
.LBB7:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/string_fortified.h"
	.loc 2 59 10 is_stmt 0 view .LVU7
	movi	v0.4s, 0
.LBE7:
.LBE6:
	.loc 1 32 89 view .LVU8
	stp	x29, x30, [sp, -384]!
	.cfi_def_cfa_offset 384
	.cfi_offset 29, -384
	.cfi_offset 30, -376
.LVL1:
.LBB16:
.LBI6:
	.loc 2 57 1 is_stmt 1 view .LVU9
.LBB8:
	.loc 2 59 3 view .LVU10
.LBE8:
.LBE16:
	.loc 1 32 89 is_stmt 0 view .LVU11
	mov	x29, sp
	stp	x23, x24, [sp, 48]
	.cfi_offset 23, -336
	.cfi_offset 24, -328
.LBB17:
.LBB9:
	.loc 2 59 10 view .LVU12
	add	x24, sp, 80
.LVL2:
	.loc 2 59 10 view .LVU13
.LBE9:
.LBE17:
.LBB18:
.LBB19:
	add	x23, sp, 232
.LBE19:
.LBE18:
	.loc 1 32 89 view .LVU14
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -368
	.cfi_offset 20, -360
	mov	x20, x1
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -352
	.cfi_offset 22, -344
	mov	x21, x2
	str	x25, [sp, 64]
	.cfi_offset 25, -320
	.loc 1 32 89 view .LVU15
	mov	x25, x0
.LBB26:
.LBB20:
	.loc 2 59 10 view .LVU16
	str	xzr, [x23, 144]
.LBE20:
.LBE26:
.LBB27:
.LBB10:
	str	xzr, [x24, 144]
.LVL3:
	.loc 2 59 10 view .LVU17
.LBE10:
.LBE27:
	.loc 1 41 2 is_stmt 1 view .LVU18
.LBB28:
.LBI18:
	.loc 2 57 1 view .LVU19
.LBB21:
	.loc 2 59 3 view .LVU20
	.loc 2 59 3 is_stmt 0 view .LVU21
.LBE21:
.LBE28:
	.loc 1 43 2 is_stmt 1 view .LVU22
.LBB29:
.LBB11:
	.loc 2 59 10 is_stmt 0 view .LVU23
	stp	q0, q0, [x24]
.LBE11:
.LBE29:
.LBB30:
.LBB22:
	stp	q0, q0, [x23]
.LBE22:
.LBE30:
.LBB31:
.LBB12:
	stp	q0, q0, [x24, 32]
.LBE12:
.LBE31:
.LBB32:
.LBB23:
	stp	q0, q0, [x23, 32]
.LBE23:
.LBE32:
.LBB33:
.LBB13:
	stp	q0, q0, [x24, 64]
.LBE13:
.LBE33:
.LBB34:
.LBB24:
	stp	q0, q0, [x23, 64]
.LBE24:
.LBE34:
.LBB35:
.LBB14:
	stp	q0, q0, [x24, 96]
.LBE14:
.LBE35:
.LBB36:
.LBB25:
	stp	q0, q0, [x23, 96]
	str	q0, [x23, 128]
.LBE25:
.LBE36:
.LBB37:
.LBB15:
	str	q0, [x24, 128]
.LBE15:
.LBE37:
	.loc 1 43 5 view .LVU24
	cbz	x1, .L2
	.loc 1 44 3 is_stmt 1 view .LVU25
	.loc 1 45 23 is_stmt 0 view .LVU26
	ldr	x1, [x1]
.LVL4:
	.loc 1 49 3 view .LVU27
	add	x22, sp, 88
	mov	x0, x22
.LVL5:
	.loc 1 50 10 view .LVU28
	mov	x19, 0
	.loc 1 45 21 view .LVU29
	str	x1, [sp, 80]
	.loc 1 49 3 is_stmt 1 view .LVU30
	bl	sigemptyset
.LVL6:
	.loc 1 50 3 view .LVU31
	.loc 1 50 17 view .LVU32
	b	.L6
.LVL7:
	.p2align 2,,3
.L5:
	.loc 1 50 45 discriminator 2 view .LVU33
	add	x19, x19, 1
.LVL8:
	.loc 1 50 17 discriminator 2 view .LVU34
	cmp	x19, 64
	beq	.L32
.LVL9:
.L6:
	.loc 1 51 4 view .LVU35
	.loc 1 51 8 is_stmt 0 view .LVU36
	ldr	x3, [x20, 24]
	lsr	x3, x3, x19
	.loc 1 51 7 view .LVU37
	tbz	x3, 0, .L5
	.loc 1 52 5 is_stmt 1 view .LVU38
	add	w1, w19, 1
	mov	x0, x22
	.loc 1 50 45 is_stmt 0 view .LVU39
	add	x19, x19, 1
.LVL10:
	.loc 1 52 5 view .LVU40
	bl	sigaddset
.LVL11:
	.loc 1 50 45 is_stmt 1 view .LVU41
	.loc 1 50 17 view .LVU42
	cmp	x19, 64
	bne	.L6
.L32:
	.loc 1 55 3 view .LVU43
	.loc 1 55 37 is_stmt 0 view .LVU44
	ldr	x0, [x20, 8]
	.loc 1 58 8 view .LVU45
	mov	x20, x24
.LVL12:
	.loc 1 55 18 view .LVU46
	and	w0, w0, -67108865
	.loc 1 55 16 view .LVU47
	str	w0, [sp, 216]
	.loc 1 58 2 is_stmt 1 view .LVU48
.LVL13:
.L2:
	.loc 1 58 8 is_stmt 0 discriminator 4 view .LVU49
	cbz	x21, .L33
	.loc 1 58 8 view .LVU50
	mov	x2, x23
	mov	x1, x20
	mov	w0, w25
	bl	sigaction
.LVL14:
	mov	w23, w0
.LVL15:
	.loc 1 59 2 is_stmt 1 view .LVU51
	.loc 1 59 5 is_stmt 0 view .LVU52
	cmn	w0, #1
	beq	.L12
	.loc 1 65 2 is_stmt 1 view .LVU53
	.loc 1 66 3 view .LVU54
	add	x20, sp, 240
	.loc 1 72 10 is_stmt 0 view .LVU55
	mov	x19, 0
	ldr	x0, [sp, 232]
.LVL16:
	.loc 1 74 36 view .LVU56
	mov	x22, 1
	str	x0, [x21]
	.loc 1 71 3 is_stmt 1 view .LVU57
	.loc 1 71 18 is_stmt 0 view .LVU58
	str	xzr, [x21, 24]
	.loc 1 72 3 is_stmt 1 view .LVU59
.LVL17:
	.loc 1 72 17 view .LVU60
	.p2align 3,,7
.L11:
	.loc 1 73 4 view .LVU61
	.loc 1 73 8 is_stmt 0 view .LVU62
	add	w1, w19, 1
	mov	x0, x20
	bl	sigismember
.LVL18:
	.loc 1 72 48 is_stmt 1 view .LVU63
	.loc 1 73 7 is_stmt 0 view .LVU64
	cmp	w0, 1
	bne	.L10
	.loc 1 74 5 is_stmt 1 view .LVU65
	.loc 1 74 20 is_stmt 0 view .LVU66
	ldr	x1, [x21, 24]
	.loc 1 74 36 view .LVU67
	lsl	x2, x22, x19
	.loc 1 74 20 view .LVU68
	orr	x1, x1, x2
	str	x1, [x21, 24]
.L10:
	.loc 1 72 48 discriminator 2 view .LVU69
	add	x19, x19, 1
.LVL19:
	.loc 1 72 17 is_stmt 1 discriminator 2 view .LVU70
	cmp	x19, 64
	bne	.L11
	.loc 1 77 3 view .LVU71
	.loc 1 77 21 is_stmt 0 view .LVU72
	ldrsw	x0, [sp, 368]
	str	x0, [x21, 8]
.LVL20:
.L1:
	.loc 1 82 1 view .LVU73
	mov	w0, w23
	ldp	x19, x20, [sp, 16]
	ldp	x21, x22, [sp, 32]
.LVL21:
	.loc 1 82 1 view .LVU74
	ldp	x23, x24, [sp, 48]
.LVL22:
	.loc 1 82 1 view .LVU75
	ldr	x25, [sp, 64]
.LVL23:
	.loc 1 82 1 view .LVU76
	ldp	x29, x30, [sp], 384
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 25
	.cfi_restore 23
	.cfi_restore 24
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL24:
.L33:
	.cfi_restore_state
	.loc 1 58 8 view .LVU77
	mov	x1, x20
	mov	w0, w25
	mov	x2, 0
	bl	sigaction
.LVL25:
	mov	w23, w0
.LVL26:
	.loc 1 59 2 is_stmt 1 view .LVU78
	.loc 1 59 5 is_stmt 0 view .LVU79
	cmn	w0, #1
	bne	.L1
.L12:
	.loc 1 61 22 is_stmt 1 view .LVU80
	.loc 1 62 3 view .LVU81
	.loc 1 62 10 is_stmt 0 view .LVU82
	bl	__errno_location
.LVL27:
	.loc 1 62 10 view .LVU83
	ldr	w23, [x0]
.LVL28:
	.loc 1 82 1 view .LVU84
	ldp	x19, x20, [sp, 16]
	mov	w0, w23
	ldp	x21, x22, [sp, 32]
.LVL29:
	.loc 1 82 1 view .LVU85
	ldp	x23, x24, [sp, 48]
	ldr	x25, [sp, 64]
.LVL30:
	.loc 1 82 1 view .LVU86
	ldp	x29, x30, [sp], 384
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 25
	.cfi_restore 23
	.cfi_restore 24
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE51:
	.size	x_cgo_sigaction, .-x_cgo_sigaction
.Letext0:
	.file 3 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 4 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 5 "/usr/include/aarch64-linux-gnu/bits/stdint-intn.h"
	.file 6 "/usr/include/aarch64-linux-gnu/bits/stdint-uintn.h"
	.file 7 "/usr/include/stdint.h"
	.file 8 "/usr/include/aarch64-linux-gnu/bits/types/__sigset_t.h"
	.file 9 "/usr/include/aarch64-linux-gnu/bits/types/sigset_t.h"
	.file 10 "/usr/include/aarch64-linux-gnu/bits/types/__sigval_t.h"
	.file 11 "/usr/include/aarch64-linux-gnu/bits/types/siginfo_t.h"
	.file 12 "/usr/include/signal.h"
	.file 13 "/usr/include/aarch64-linux-gnu/bits/sigaction.h"
	.file 14 "/usr/include/errno.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x76c
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x16
	.4byte	.LASF88
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x4
	.byte	0x8
	.byte	0x5
	.4byte	.LASF2
	.uleb128 0x2
	.4byte	.LASF11
	.byte	0x3
	.byte	0xd1
	.byte	0x17
	.4byte	0x41
	.uleb128 0x4
	.byte	0x8
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x4
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x4
	.byte	0x8
	.byte	0x5
	.4byte	.LASF5
	.uleb128 0x4
	.byte	0x10
	.byte	0x4
	.4byte	.LASF6
	.uleb128 0x4
	.byte	0x1
	.byte	0x8
	.4byte	.LASF7
	.uleb128 0x4
	.byte	0x2
	.byte	0x7
	.4byte	.LASF8
	.uleb128 0x4
	.byte	0x1
	.byte	0x6
	.4byte	.LASF9
	.uleb128 0x4
	.byte	0x2
	.byte	0x5
	.4byte	.LASF10
	.uleb128 0x2
	.4byte	.LASF12
	.byte	0x4
	.byte	0x29
	.byte	0x14
	.4byte	0x85
	.uleb128 0x17
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x2
	.4byte	.LASF13
	.byte	0x4
	.byte	0x2a
	.byte	0x16
	.4byte	0x48
	.uleb128 0x2
	.4byte	.LASF14
	.byte	0x4
	.byte	0x2d
	.byte	0x1b
	.4byte	0x41
	.uleb128 0x2
	.4byte	.LASF15
	.byte	0x4
	.byte	0x92
	.byte	0x19
	.4byte	0x48
	.uleb128 0x2
	.4byte	.LASF16
	.byte	0x4
	.byte	0x9a
	.byte	0x19
	.4byte	0x85
	.uleb128 0x2
	.4byte	.LASF17
	.byte	0x4
	.byte	0x9c
	.byte	0x1b
	.4byte	0x2e
	.uleb128 0x18
	.byte	0x8
	.uleb128 0x4
	.byte	0x1
	.byte	0x8
	.4byte	.LASF18
	.uleb128 0x2
	.4byte	.LASF19
	.byte	0x5
	.byte	0x1a
	.byte	0x13
	.4byte	0x79
	.uleb128 0x2
	.4byte	.LASF20
	.byte	0x6
	.byte	0x1b
	.byte	0x14
	.4byte	0x98
	.uleb128 0x2
	.4byte	.LASF21
	.byte	0x7
	.byte	0x57
	.byte	0x13
	.4byte	0x2e
	.uleb128 0x2
	.4byte	.LASF22
	.byte	0x7
	.byte	0x5a
	.byte	0x1b
	.4byte	0x41
	.uleb128 0x6
	.byte	0x80
	.byte	0x8
	.byte	0x5
	.byte	0x9
	.4byte	0x118
	.uleb128 0x1
	.4byte	.LASF28
	.byte	0x8
	.byte	0x7
	.byte	0x15
	.4byte	0x118
	.byte	0
	.byte	0
	.uleb128 0x10
	.4byte	0x41
	.4byte	0x128
	.uleb128 0x11
	.4byte	0x41
	.byte	0xf
	.byte	0
	.uleb128 0x2
	.4byte	.LASF23
	.byte	0x8
	.byte	0x8
	.byte	0x3
	.4byte	0x101
	.uleb128 0x2
	.4byte	.LASF24
	.byte	0x9
	.byte	0x7
	.byte	0x14
	.4byte	0x128
	.uleb128 0xc
	.4byte	0x134
	.uleb128 0x19
	.4byte	.LASF89
	.byte	0x8
	.byte	0xa
	.byte	0x18
	.byte	0x7
	.4byte	0x16b
	.uleb128 0x3
	.4byte	.LASF25
	.byte	0xa
	.byte	0x1a
	.byte	0x7
	.4byte	0x85
	.uleb128 0x3
	.4byte	.LASF26
	.byte	0xa
	.byte	0x1b
	.byte	0x9
	.4byte	0xc8
	.byte	0
	.uleb128 0x2
	.4byte	.LASF27
	.byte	0xa
	.byte	0x1e
	.byte	0x16
	.4byte	0x145
	.uleb128 0x6
	.byte	0x8
	.byte	0xb
	.byte	0x38
	.byte	0x2
	.4byte	0x19b
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0xb
	.byte	0x3a
	.byte	0xe
	.4byte	0xb0
	.byte	0
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0xb
	.byte	0x3b
	.byte	0xe
	.4byte	0xa4
	.byte	0x4
	.byte	0
	.uleb128 0x6
	.byte	0x10
	.byte	0xb
	.byte	0x3f
	.byte	0x2
	.4byte	0x1cc
	.uleb128 0x1
	.4byte	.LASF31
	.byte	0xb
	.byte	0x41
	.byte	0xa
	.4byte	0x85
	.byte	0
	.uleb128 0x1
	.4byte	.LASF32
	.byte	0xb
	.byte	0x42
	.byte	0xa
	.4byte	0x85
	.byte	0x4
	.uleb128 0x1
	.4byte	.LASF33
	.byte	0xb
	.byte	0x43
	.byte	0x11
	.4byte	0x16b
	.byte	0x8
	.byte	0
	.uleb128 0x6
	.byte	0x10
	.byte	0xb
	.byte	0x47
	.byte	0x2
	.4byte	0x1fd
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0xb
	.byte	0x49
	.byte	0xe
	.4byte	0xb0
	.byte	0
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0xb
	.byte	0x4a
	.byte	0xe
	.4byte	0xa4
	.byte	0x4
	.uleb128 0x1
	.4byte	.LASF33
	.byte	0xb
	.byte	0x4b
	.byte	0x11
	.4byte	0x16b
	.byte	0x8
	.byte	0
	.uleb128 0x6
	.byte	0x20
	.byte	0xb
	.byte	0x4f
	.byte	0x2
	.4byte	0x248
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0xb
	.byte	0x51
	.byte	0xe
	.4byte	0xb0
	.byte	0
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0xb
	.byte	0x52
	.byte	0xe
	.4byte	0xa4
	.byte	0x4
	.uleb128 0x1
	.4byte	.LASF34
	.byte	0xb
	.byte	0x53
	.byte	0xa
	.4byte	0x85
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF35
	.byte	0xb
	.byte	0x54
	.byte	0x13
	.4byte	0xbc
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF36
	.byte	0xb
	.byte	0x55
	.byte	0x13
	.4byte	0xbc
	.byte	0x18
	.byte	0
	.uleb128 0x6
	.byte	0x10
	.byte	0xb
	.byte	0x61
	.byte	0x3
	.4byte	0x26c
	.uleb128 0x1
	.4byte	.LASF37
	.byte	0xb
	.byte	0x63
	.byte	0xd
	.4byte	0xc8
	.byte	0
	.uleb128 0x1
	.4byte	.LASF38
	.byte	0xb
	.byte	0x64
	.byte	0xd
	.4byte	0xc8
	.byte	0x8
	.byte	0
	.uleb128 0xd
	.byte	0x10
	.byte	0xb
	.byte	0x5e
	.byte	0x6
	.4byte	0x28e
	.uleb128 0x3
	.4byte	.LASF39
	.byte	0xb
	.byte	0x65
	.byte	0x7
	.4byte	0x248
	.uleb128 0x3
	.4byte	.LASF40
	.byte	0xb
	.byte	0x67
	.byte	0xe
	.4byte	0x8c
	.byte	0
	.uleb128 0x6
	.byte	0x20
	.byte	0xb
	.byte	0x59
	.byte	0x2
	.4byte	0x2bf
	.uleb128 0x1
	.4byte	.LASF41
	.byte	0xb
	.byte	0x5b
	.byte	0xc
	.4byte	0xc8
	.byte	0
	.uleb128 0x1
	.4byte	.LASF42
	.byte	0xb
	.byte	0x5d
	.byte	0x10
	.4byte	0x72
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF43
	.byte	0xb
	.byte	0x68
	.byte	0xa
	.4byte	0x26c
	.byte	0x10
	.byte	0
	.uleb128 0x6
	.byte	0x10
	.byte	0xb
	.byte	0x6c
	.byte	0x2
	.4byte	0x2e3
	.uleb128 0x1
	.4byte	.LASF44
	.byte	0xb
	.byte	0x6e
	.byte	0x15
	.4byte	0x2e
	.byte	0
	.uleb128 0x1
	.4byte	.LASF45
	.byte	0xb
	.byte	0x6f
	.byte	0xa
	.4byte	0x85
	.byte	0x8
	.byte	0
	.uleb128 0x6
	.byte	0x10
	.byte	0xb
	.byte	0x74
	.byte	0x2
	.4byte	0x314
	.uleb128 0x1
	.4byte	.LASF46
	.byte	0xb
	.byte	0x76
	.byte	0xc
	.4byte	0xc8
	.byte	0
	.uleb128 0x1
	.4byte	.LASF47
	.byte	0xb
	.byte	0x77
	.byte	0xa
	.4byte	0x85
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF48
	.byte	0xb
	.byte	0x78
	.byte	0x13
	.4byte	0x48
	.byte	0xc
	.byte	0
	.uleb128 0xd
	.byte	0x70
	.byte	0xb
	.byte	0x33
	.byte	0x5
	.4byte	0x37e
	.uleb128 0x3
	.4byte	.LASF49
	.byte	0xb
	.byte	0x35
	.byte	0x6
	.4byte	0x37e
	.uleb128 0x3
	.4byte	.LASF50
	.byte	0xb
	.byte	0x3c
	.byte	0x6
	.4byte	0x177
	.uleb128 0x3
	.4byte	.LASF51
	.byte	0xb
	.byte	0x44
	.byte	0x6
	.4byte	0x19b
	.uleb128 0x1a
	.string	"_rt"
	.byte	0xb
	.byte	0x4c
	.byte	0x6
	.4byte	0x1cc
	.uleb128 0x3
	.4byte	.LASF52
	.byte	0xb
	.byte	0x56
	.byte	0x6
	.4byte	0x1fd
	.uleb128 0x3
	.4byte	.LASF53
	.byte	0xb
	.byte	0x69
	.byte	0x6
	.4byte	0x28e
	.uleb128 0x3
	.4byte	.LASF54
	.byte	0xb
	.byte	0x70
	.byte	0x6
	.4byte	0x2bf
	.uleb128 0x3
	.4byte	.LASF55
	.byte	0xb
	.byte	0x79
	.byte	0x6
	.4byte	0x2e3
	.byte	0
	.uleb128 0x10
	.4byte	0x85
	.4byte	0x38e
	.uleb128 0x11
	.4byte	0x41
	.byte	0x1b
	.byte	0
	.uleb128 0x6
	.byte	0x80
	.byte	0xb
	.byte	0x24
	.byte	0x9
	.4byte	0x3d9
	.uleb128 0x1
	.4byte	.LASF56
	.byte	0xb
	.byte	0x26
	.byte	0x9
	.4byte	0x85
	.byte	0
	.uleb128 0x1
	.4byte	.LASF57
	.byte	0xb
	.byte	0x28
	.byte	0x9
	.4byte	0x85
	.byte	0x4
	.uleb128 0x1
	.4byte	.LASF58
	.byte	0xb
	.byte	0x2a
	.byte	0x9
	.4byte	0x85
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF59
	.byte	0xb
	.byte	0x30
	.byte	0x9
	.4byte	0x85
	.byte	0xc
	.uleb128 0x1
	.4byte	.LASF60
	.byte	0xb
	.byte	0x7b
	.byte	0x9
	.4byte	0x314
	.byte	0x10
	.byte	0
	.uleb128 0x2
	.4byte	.LASF61
	.byte	0xb
	.byte	0x7c
	.byte	0x5
	.4byte	0x38e
	.uleb128 0x2
	.4byte	.LASF62
	.byte	0xc
	.byte	0x48
	.byte	0x10
	.4byte	0x3f1
	.uleb128 0x7
	.4byte	0x3f6
	.uleb128 0x12
	.4byte	0x401
	.uleb128 0x5
	.4byte	0x85
	.byte	0
	.uleb128 0xd
	.byte	0x8
	.byte	0xd
	.byte	0x1f
	.byte	0x5
	.4byte	0x423
	.uleb128 0x3
	.4byte	.LASF63
	.byte	0xd
	.byte	0x22
	.byte	0x11
	.4byte	0x3e5
	.uleb128 0x3
	.4byte	.LASF64
	.byte	0xd
	.byte	0x24
	.byte	0x9
	.4byte	0x43d
	.byte	0
	.uleb128 0x12
	.4byte	0x438
	.uleb128 0x5
	.4byte	0x85
	.uleb128 0x5
	.4byte	0x438
	.uleb128 0x5
	.4byte	0xc8
	.byte	0
	.uleb128 0x7
	.4byte	0x3d9
	.uleb128 0x7
	.4byte	0x423
	.uleb128 0x1b
	.4byte	.LASF77
	.byte	0x98
	.byte	0xd
	.byte	0x1b
	.byte	0x8
	.4byte	0x484
	.uleb128 0x1
	.4byte	.LASF65
	.byte	0xd
	.byte	0x26
	.byte	0x5
	.4byte	0x401
	.byte	0
	.uleb128 0x1
	.4byte	.LASF66
	.byte	0xd
	.byte	0x2e
	.byte	0x10
	.4byte	0x128
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF67
	.byte	0xd
	.byte	0x31
	.byte	0x9
	.4byte	0x85
	.byte	0x88
	.uleb128 0x1
	.4byte	.LASF68
	.byte	0xd
	.byte	0x34
	.byte	0xc
	.4byte	0x48a
	.byte	0x90
	.byte	0
	.uleb128 0xc
	.4byte	0x442
	.uleb128 0x1c
	.uleb128 0x7
	.4byte	0x489
	.uleb128 0x4
	.byte	0x8
	.byte	0x7
	.4byte	.LASF69
	.uleb128 0x4
	.byte	0x10
	.byte	0x7
	.4byte	.LASF70
	.uleb128 0x6
	.byte	0x20
	.byte	0x1
	.byte	0x12
	.byte	0x9
	.4byte	0x4db
	.uleb128 0x1
	.4byte	.LASF71
	.byte	0x1
	.byte	0x13
	.byte	0xc
	.4byte	0xf5
	.byte	0
	.uleb128 0x1
	.4byte	.LASF72
	.byte	0x1
	.byte	0x14
	.byte	0xb
	.4byte	0xdd
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF73
	.byte	0x1
	.byte	0x15
	.byte	0xc
	.4byte	0xf5
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF74
	.byte	0x1
	.byte	0x16
	.byte	0xb
	.4byte	0xdd
	.byte	0x18
	.byte	0
	.uleb128 0x2
	.4byte	.LASF75
	.byte	0x1
	.byte	0x17
	.byte	0x3
	.4byte	0x49d
	.uleb128 0xc
	.4byte	0x4db
	.uleb128 0xb
	.4byte	.LASF76
	.byte	0xd3
	.4byte	0x85
	.4byte	0x505
	.uleb128 0x5
	.4byte	0x505
	.uleb128 0x5
	.4byte	0x85
	.byte	0
	.uleb128 0x7
	.4byte	0x140
	.uleb128 0x1d
	.4byte	.LASF90
	.byte	0xe
	.byte	0x25
	.byte	0xd
	.4byte	0x516
	.uleb128 0x7
	.4byte	0x85
	.uleb128 0xb
	.4byte	.LASF77
	.byte	0xf3
	.4byte	0x85
	.4byte	0x539
	.uleb128 0x5
	.4byte	0x85
	.uleb128 0x5
	.4byte	0x53e
	.uleb128 0x5
	.4byte	0x548
	.byte	0
	.uleb128 0x7
	.4byte	0x484
	.uleb128 0x13
	.4byte	0x539
	.uleb128 0x7
	.4byte	0x442
	.uleb128 0x13
	.4byte	0x543
	.uleb128 0xb
	.4byte	.LASF78
	.byte	0xcd
	.4byte	0x85
	.4byte	0x566
	.uleb128 0x5
	.4byte	0x566
	.uleb128 0x5
	.4byte	0x85
	.byte	0
	.uleb128 0x7
	.4byte	0x134
	.uleb128 0xb
	.4byte	.LASF79
	.byte	0xc7
	.4byte	0x85
	.4byte	0x57f
	.uleb128 0x5
	.4byte	0x566
	.byte	0
	.uleb128 0x1e
	.4byte	.LASF91
	.byte	0x1
	.byte	0x20
	.byte	0x1
	.4byte	0xd1
	.8byte	.LFB51
	.8byte	.LFE51-.LFB51
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x73c
	.uleb128 0xe
	.4byte	.LASF80
	.byte	0x1a
	.4byte	0xe9
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0xe
	.4byte	.LASF81
	.byte	0x38
	.4byte	0x73c
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0xe
	.4byte	.LASF82
	.byte	0x4f
	.4byte	0x741
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x14
	.string	"ret"
	.byte	0x21
	.byte	0xa
	.4byte	0xd1
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x1f
	.string	"act"
	.byte	0x1
	.byte	0x22
	.byte	0x13
	.4byte	0x442
	.uleb128 0x3
	.byte	0x91
	.sleb128 -304
	.uleb128 0x20
	.4byte	.LASF83
	.byte	0x1
	.byte	0x23
	.byte	0x13
	.4byte	0x442
	.uleb128 0x3
	.byte	0x91
	.sleb128 -152
	.uleb128 0x14
	.string	"i"
	.byte	0x24
	.byte	0x9
	.4byte	0x35
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x15
	.4byte	0x746
	.8byte	.LBI6
	.byte	.LVU9
	.4byte	.LLRL5
	.byte	0x28
	.4byte	0x65a
	.uleb128 0x9
	.4byte	0x765
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x9
	.4byte	0x75c
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x9
	.4byte	0x753
	.4byte	.LLST8
	.4byte	.LVUS8
	.byte	0
	.uleb128 0x15
	.4byte	0x746
	.8byte	.LBI18
	.byte	.LVU19
	.4byte	.LLRL9
	.byte	0x29
	.4byte	0x699
	.uleb128 0x9
	.4byte	0x765
	.4byte	.LLST10
	.4byte	.LVUS10
	.uleb128 0x9
	.4byte	0x75c
	.4byte	.LLST11
	.4byte	.LVUS11
	.uleb128 0x9
	.4byte	0x753
	.4byte	.LLST12
	.4byte	.LVUS12
	.byte	0
	.uleb128 0xa
	.8byte	.LVL6
	.4byte	0x56b
	.4byte	0x6b1
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x86
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL11
	.4byte	0x54d
	.4byte	0x6c9
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x86
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL14
	.4byte	0x51b
	.4byte	0x6ed
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x89
	.sleb128 0
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x87
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL18
	.4byte	0x4ec
	.4byte	0x70b
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x83
	.sleb128 1
	.byte	0
	.uleb128 0xa
	.8byte	.LVL25
	.4byte	0x51b
	.4byte	0x72e
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x89
	.sleb128 0
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x8
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.byte	0x30
	.byte	0
	.uleb128 0x21
	.8byte	.LVL27
	.4byte	0x50a
	.byte	0
	.uleb128 0x7
	.4byte	0x4e7
	.uleb128 0x7
	.4byte	0x4db
	.uleb128 0x22
	.4byte	.LASF84
	.byte	0x2
	.byte	0x39
	.byte	0x1
	.4byte	0xc8
	.byte	0x3
	.uleb128 0xf
	.4byte	.LASF85
	.4byte	0xc8
	.uleb128 0xf
	.4byte	.LASF86
	.4byte	0x85
	.uleb128 0xf
	.4byte	.LASF87
	.4byte	0x35
	.byte	0
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
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
	.uleb128 0x2
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
	.uleb128 0xe
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
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
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
	.uleb128 0xb
	.uleb128 0x2e
	.byte	0x1
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 12
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
	.uleb128 0xc
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xd
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
	.uleb128 0xe
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 32
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
	.uleb128 0xf
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0x21
	.sleb128 57
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x11
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x12
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x13
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x14
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
	.uleb128 0x15
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
	.uleb128 0x16
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
	.uleb128 0x17
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
	.uleb128 0x18
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x19
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
	.uleb128 0x1a
	.uleb128 0xd
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
	.byte	0
	.byte	0
	.uleb128 0x1b
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
	.uleb128 0x1c
	.uleb128 0x15
	.byte	0
	.uleb128 0x27
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
	.uleb128 0x1f
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
	.uleb128 0x20
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
	.uleb128 0x2
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x21
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x22
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
	.uleb128 0x20
	.uleb128 0xb
	.uleb128 0x34
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
	.uleb128 .LVU28
	.uleb128 .LVU28
	.uleb128 .LVU76
	.uleb128 .LVU76
	.uleb128 .LVU77
	.uleb128 .LVU77
	.uleb128 .LVU86
	.uleb128 .LVU86
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LVL23-.Ltext0
	.uleb128 0x1
	.byte	0x69
	.byte	0x4
	.uleb128 .LVL23-.Ltext0
	.uleb128 .LVL24-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL24-.Ltext0
	.uleb128 .LVL30-.Ltext0
	.uleb128 0x1
	.byte	0x69
	.byte	0x4
	.uleb128 .LVL30-.Ltext0
	.uleb128 .LFE51-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 0
	.uleb128 .LVU27
	.uleb128 .LVU27
	.uleb128 .LVU46
	.uleb128 .LVU46
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL12-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL12-.Ltext0
	.uleb128 .LFE51-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU31
	.uleb128 .LVU31
	.uleb128 .LVU74
	.uleb128 .LVU74
	.uleb128 .LVU77
	.uleb128 .LVU77
	.uleb128 .LVU85
	.uleb128 .LVU85
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL6-1-.Ltext0
	.uleb128 0x1
	.byte	0x52
	.byte	0x4
	.uleb128 .LVL6-1-.Ltext0
	.uleb128 .LVL21-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL24-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL24-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LFE51-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0
.LVUS3:
	.uleb128 .LVU51
	.uleb128 .LVU56
	.uleb128 .LVU56
	.uleb128 .LVU75
	.uleb128 .LVU75
	.uleb128 .LVU77
	.uleb128 .LVU78
	.uleb128 .LVU83
	.uleb128 .LVU83
	.uleb128 .LVU84
.LLST3:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL16-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x1
	.byte	0x67
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LVL24-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL26-.Ltext0
	.uleb128 .LVL27-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL27-1-.Ltext0
	.uleb128 .LVL28-.Ltext0
	.uleb128 0x1
	.byte	0x67
	.byte	0
.LVUS4:
	.uleb128 .LVU32
	.uleb128 .LVU33
	.uleb128 .LVU33
	.uleb128 .LVU40
	.uleb128 .LVU40
	.uleb128 .LVU42
	.uleb128 .LVU42
	.uleb128 .LVU49
	.uleb128 .LVU60
	.uleb128 .LVU61
	.uleb128 .LVU61
	.uleb128 .LVU73
.LLST4:
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL7-.Ltext0
	.uleb128 0x2
	.byte	0x30
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LVL10-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL10-.Ltext0
	.uleb128 .LVL11-.Ltext0
	.uleb128 0x3
	.byte	0x83
	.sleb128 -1
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL11-.Ltext0
	.uleb128 .LVL13-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL17-.Ltext0
	.uleb128 .LVL17-.Ltext0
	.uleb128 0x2
	.byte	0x30
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL17-.Ltext0
	.uleb128 .LVL20-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS6:
	.uleb128 .LVU9
	.uleb128 .LVU17
.LLST6:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x3
	.byte	0x8
	.byte	0x98
	.byte	0x9f
	.byte	0
.LVUS7:
	.uleb128 .LVU9
	.uleb128 .LVU17
.LLST7:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x2
	.byte	0x30
	.byte	0x9f
	.byte	0
.LVUS8:
	.uleb128 .LVU9
	.uleb128 .LVU13
	.uleb128 .LVU13
	.uleb128 .LVU17
.LLST8:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x4
	.byte	0x91
	.sleb128 -304
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x1
	.byte	0x68
	.byte	0
.LVUS10:
	.uleb128 .LVU19
	.uleb128 .LVU21
.LLST10:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x3
	.byte	0x8
	.byte	0x98
	.byte	0x9f
	.byte	0
.LVUS11:
	.uleb128 .LVU19
	.uleb128 .LVU21
.LLST11:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x2
	.byte	0x30
	.byte	0x9f
	.byte	0
.LVUS12:
	.uleb128 .LVU19
	.uleb128 .LVU21
.LLST12:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x1
	.byte	0x67
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
.LLRL5:
	.byte	0x4
	.uleb128 .LBB6-.Ltext0
	.uleb128 .LBE6-.Ltext0
	.byte	0x4
	.uleb128 .LBB16-.Ltext0
	.uleb128 .LBE16-.Ltext0
	.byte	0x4
	.uleb128 .LBB17-.Ltext0
	.uleb128 .LBE17-.Ltext0
	.byte	0x4
	.uleb128 .LBB27-.Ltext0
	.uleb128 .LBE27-.Ltext0
	.byte	0x4
	.uleb128 .LBB29-.Ltext0
	.uleb128 .LBE29-.Ltext0
	.byte	0x4
	.uleb128 .LBB31-.Ltext0
	.uleb128 .LBE31-.Ltext0
	.byte	0x4
	.uleb128 .LBB33-.Ltext0
	.uleb128 .LBE33-.Ltext0
	.byte	0x4
	.uleb128 .LBB35-.Ltext0
	.uleb128 .LBE35-.Ltext0
	.byte	0x4
	.uleb128 .LBB37-.Ltext0
	.uleb128 .LBE37-.Ltext0
	.byte	0
.LLRL9:
	.byte	0x4
	.uleb128 .LBB18-.Ltext0
	.uleb128 .LBE18-.Ltext0
	.byte	0x4
	.uleb128 .LBB26-.Ltext0
	.uleb128 .LBE26-.Ltext0
	.byte	0x4
	.uleb128 .LBB28-.Ltext0
	.uleb128 .LBE28-.Ltext0
	.byte	0x4
	.uleb128 .LBB30-.Ltext0
	.uleb128 .LBE30-.Ltext0
	.byte	0x4
	.uleb128 .LBB32-.Ltext0
	.uleb128 .LBE32-.Ltext0
	.byte	0x4
	.uleb128 .LBB34-.Ltext0
	.uleb128 .LBE34-.Ltext0
	.byte	0x4
	.uleb128 .LBB36-.Ltext0
	.uleb128 .LBE36-.Ltext0
	.byte	0
.Ldebug_ranges3:
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF90:
	.string	"__errno_location"
.LASF17:
	.string	"__clock_t"
.LASF73:
	.string	"restorer"
.LASF91:
	.string	"x_cgo_sigaction"
.LASF14:
	.string	"__uint64_t"
.LASF43:
	.string	"_bounds"
.LASF70:
	.string	"__int128 unsigned"
.LASF20:
	.string	"uint64_t"
.LASF10:
	.string	"short int"
.LASF11:
	.string	"size_t"
.LASF21:
	.string	"intptr_t"
.LASF64:
	.string	"sa_sigaction"
.LASF86:
	.string	"__ch"
.LASF16:
	.string	"__pid_t"
.LASF48:
	.string	"_arch"
.LASF78:
	.string	"sigaddset"
.LASF75:
	.string	"go_sigaction_t"
.LASF36:
	.string	"si_stime"
.LASF34:
	.string	"si_status"
.LASF38:
	.string	"_upper"
.LASF71:
	.string	"handler"
.LASF50:
	.string	"_kill"
.LASF32:
	.string	"si_overrun"
.LASF77:
	.string	"sigaction"
.LASF41:
	.string	"si_addr"
.LASF63:
	.string	"sa_handler"
.LASF55:
	.string	"_sigsys"
.LASF22:
	.string	"uintptr_t"
.LASF35:
	.string	"si_utime"
.LASF33:
	.string	"si_sigval"
.LASF5:
	.string	"long long int"
.LASF84:
	.string	"memset"
.LASF51:
	.string	"_timer"
.LASF2:
	.string	"long int"
.LASF85:
	.string	"__dest"
.LASF26:
	.string	"sival_ptr"
.LASF76:
	.string	"sigismember"
.LASF47:
	.string	"_syscall"
.LASF58:
	.string	"si_code"
.LASF44:
	.string	"si_band"
.LASF40:
	.string	"_pkey"
.LASF56:
	.string	"si_signo"
.LASF39:
	.string	"_addr_bnd"
.LASF6:
	.string	"long double"
.LASF60:
	.string	"_sifields"
.LASF7:
	.string	"unsigned char"
.LASF62:
	.string	"__sighandler_t"
.LASF9:
	.string	"signed char"
.LASF72:
	.string	"flags"
.LASF69:
	.string	"long long unsigned int"
.LASF4:
	.string	"unsigned int"
.LASF88:
	.string	"GNU C17 11.4.0"
.LASF54:
	.string	"_sigpoll"
.LASF81:
	.string	"goact"
.LASF57:
	.string	"si_errno"
.LASF8:
	.string	"short unsigned int"
.LASF49:
	.string	"_pad"
.LASF18:
	.string	"char"
.LASF79:
	.string	"sigemptyset"
.LASF28:
	.string	"__val"
.LASF19:
	.string	"int32_t"
.LASF30:
	.string	"si_uid"
.LASF46:
	.string	"_call_addr"
.LASF59:
	.string	"__pad0"
.LASF53:
	.string	"_sigfault"
.LASF66:
	.string	"sa_mask"
.LASF83:
	.string	"oldact"
.LASF89:
	.string	"sigval"
.LASF42:
	.string	"si_addr_lsb"
.LASF31:
	.string	"si_tid"
.LASF3:
	.string	"long unsigned int"
.LASF29:
	.string	"si_pid"
.LASF68:
	.string	"sa_restorer"
.LASF82:
	.string	"oldgoact"
.LASF65:
	.string	"__sigaction_handler"
.LASF13:
	.string	"__uint32_t"
.LASF67:
	.string	"sa_flags"
.LASF74:
	.string	"mask"
.LASF12:
	.string	"__int32_t"
.LASF25:
	.string	"sival_int"
.LASF24:
	.string	"sigset_t"
.LASF87:
	.string	"__len"
.LASF15:
	.string	"__uid_t"
.LASF27:
	.string	"__sigval_t"
.LASF45:
	.string	"si_fd"
.LASF37:
	.string	"_lower"
.LASF52:
	.string	"_sigchld"
.LASF61:
	.string	"siginfo_t"
.LASF80:
	.string	"signum"
.LASF23:
	.string	"__sigset_t"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_sigaction.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
