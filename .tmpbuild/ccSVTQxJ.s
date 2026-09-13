	.arch armv8-a
	.file	"linux_syscall.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "linux_syscall.c"
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setegid
	.type	_cgo_libc_setegid, %function
_cgo_libc_setegid:
.LVL0:
.LFB59:
	.file 1 "linux_syscall.c"
	.loc 1 41 32 view -0
	.cfi_startproc
	.loc 1 42 2 view .LVU1
	.loc 1 41 32 is_stmt 0 view .LVU2
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 41 32 view .LVU3
	mov	x19, x0
	.loc 1 42 2 view .LVU4
	ldr	x0, [x0]
.LVL1:
	.loc 1 42 2 view .LVU5
	ldr	w0, [x0]
	bl	setegid
.LVL2:
	.loc 1 42 2 is_stmt 1 view .LVU6
	sxtw	x1, w0
.LVL3:
	.loc 1 42 2 is_stmt 0 view .LVU7
	cmn	w0, #1
	bne	.L2
	.loc 1 42 2 is_stmt 1 discriminator 1 view .LVU8
	bl	__errno_location
.LVL4:
	.loc 1 42 2 is_stmt 0 discriminator 1 view .LVU9
	ldrsw	x1, [x0]
.L2:
	str	x1, [x19, 8]
	.loc 1 43 1 view .LVU10
	ldr	x19, [sp, 16]
.LVL5:
	.loc 1 43 1 view .LVU11
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE59:
	.size	_cgo_libc_setegid, .-_cgo_libc_setegid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_seteuid
	.type	_cgo_libc_seteuid, %function
_cgo_libc_seteuid:
.LVL6:
.LFB60:
	.loc 1 46 32 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 47 2 view .LVU13
	.loc 1 46 32 is_stmt 0 view .LVU14
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 46 32 view .LVU15
	mov	x19, x0
	.loc 1 47 2 view .LVU16
	ldr	x0, [x0]
.LVL7:
	.loc 1 47 2 view .LVU17
	ldr	w0, [x0]
	bl	seteuid
.LVL8:
	.loc 1 47 2 is_stmt 1 view .LVU18
	sxtw	x1, w0
.LVL9:
	.loc 1 47 2 is_stmt 0 view .LVU19
	cmn	w0, #1
	bne	.L7
	.loc 1 47 2 is_stmt 1 discriminator 1 view .LVU20
	bl	__errno_location
.LVL10:
	.loc 1 47 2 is_stmt 0 discriminator 1 view .LVU21
	ldrsw	x1, [x0]
.L7:
	str	x1, [x19, 8]
	.loc 1 48 1 view .LVU22
	ldr	x19, [sp, 16]
.LVL11:
	.loc 1 48 1 view .LVU23
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE60:
	.size	_cgo_libc_seteuid, .-_cgo_libc_seteuid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setgid
	.type	_cgo_libc_setgid, %function
_cgo_libc_setgid:
.LVL12:
.LFB61:
	.loc 1 51 31 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 52 2 view .LVU25
	.loc 1 51 31 is_stmt 0 view .LVU26
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 51 31 view .LVU27
	mov	x19, x0
	.loc 1 52 2 view .LVU28
	ldr	x0, [x0]
.LVL13:
	.loc 1 52 2 view .LVU29
	ldr	w0, [x0]
	bl	setgid
.LVL14:
	.loc 1 52 2 is_stmt 1 view .LVU30
	sxtw	x1, w0
.LVL15:
	.loc 1 52 2 is_stmt 0 view .LVU31
	cmn	w0, #1
	bne	.L11
	.loc 1 52 2 is_stmt 1 discriminator 1 view .LVU32
	bl	__errno_location
.LVL16:
	.loc 1 52 2 is_stmt 0 discriminator 1 view .LVU33
	ldrsw	x1, [x0]
.L11:
	str	x1, [x19, 8]
	.loc 1 53 1 view .LVU34
	ldr	x19, [sp, 16]
.LVL17:
	.loc 1 53 1 view .LVU35
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE61:
	.size	_cgo_libc_setgid, .-_cgo_libc_setgid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setgroups
	.type	_cgo_libc_setgroups, %function
_cgo_libc_setgroups:
.LVL18:
.LFB62:
	.loc 1 56 34 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 57 2 view .LVU37
	.loc 1 56 34 is_stmt 0 view .LVU38
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	.loc 1 57 2 view .LVU39
	ldr	x1, [x0]
	.loc 1 56 34 view .LVU40
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 56 34 view .LVU41
	mov	x19, x0
	.loc 1 57 2 view .LVU42
	ldp	x0, x1, [x1]
.LVL19:
	.loc 1 57 2 view .LVU43
	bl	setgroups
.LVL20:
	.loc 1 57 2 is_stmt 1 view .LVU44
	sxtw	x1, w0
.LVL21:
	.loc 1 57 2 is_stmt 0 view .LVU45
	cmn	w0, #1
	bne	.L15
	.loc 1 57 2 is_stmt 1 discriminator 1 view .LVU46
	bl	__errno_location
.LVL22:
	.loc 1 57 2 is_stmt 0 discriminator 1 view .LVU47
	ldrsw	x1, [x0]
.L15:
	str	x1, [x19, 8]
	.loc 1 58 1 view .LVU48
	ldr	x19, [sp, 16]
.LVL23:
	.loc 1 58 1 view .LVU49
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE62:
	.size	_cgo_libc_setgroups, .-_cgo_libc_setgroups
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setregid
	.type	_cgo_libc_setregid, %function
_cgo_libc_setregid:
.LVL24:
.LFB63:
	.loc 1 61 33 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 62 2 view .LVU51
	.loc 1 61 33 is_stmt 0 view .LVU52
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	.loc 1 62 2 view .LVU53
	ldr	x1, [x0]
	.loc 1 61 33 view .LVU54
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 61 33 view .LVU55
	mov	x19, x0
	.loc 1 62 2 view .LVU56
	ldr	w0, [x1]
.LVL25:
	.loc 1 62 2 view .LVU57
	ldr	w1, [x1, 8]
	bl	setregid
.LVL26:
	.loc 1 62 2 is_stmt 1 view .LVU58
	sxtw	x1, w0
.LVL27:
	.loc 1 62 2 is_stmt 0 view .LVU59
	cmn	w0, #1
	bne	.L19
	.loc 1 62 2 is_stmt 1 discriminator 1 view .LVU60
	bl	__errno_location
.LVL28:
	.loc 1 62 2 is_stmt 0 discriminator 1 view .LVU61
	ldrsw	x1, [x0]
.L19:
	str	x1, [x19, 8]
	.loc 1 63 1 view .LVU62
	ldr	x19, [sp, 16]
.LVL29:
	.loc 1 63 1 view .LVU63
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE63:
	.size	_cgo_libc_setregid, .-_cgo_libc_setregid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setresgid
	.type	_cgo_libc_setresgid, %function
_cgo_libc_setresgid:
.LVL30:
.LFB64:
	.loc 1 66 34 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 67 2 view .LVU65
	.loc 1 66 34 is_stmt 0 view .LVU66
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	.loc 1 67 2 view .LVU67
	ldr	x2, [x0]
	ldr	w1, [x2, 8]
	.loc 1 66 34 view .LVU68
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 66 34 view .LVU69
	mov	x19, x0
	.loc 1 67 2 view .LVU70
	ldr	w0, [x2]
.LVL31:
	.loc 1 67 2 view .LVU71
	ldr	w2, [x2, 16]
	bl	setresgid
.LVL32:
	.loc 1 67 2 is_stmt 1 view .LVU72
	sxtw	x1, w0
.LVL33:
	.loc 1 67 2 is_stmt 0 view .LVU73
	cmn	w0, #1
	bne	.L23
	.loc 1 67 2 is_stmt 1 discriminator 1 view .LVU74
	bl	__errno_location
.LVL34:
	.loc 1 67 2 is_stmt 0 discriminator 1 view .LVU75
	ldrsw	x1, [x0]
.L23:
	str	x1, [x19, 8]
	.loc 1 69 1 view .LVU76
	ldr	x19, [sp, 16]
.LVL35:
	.loc 1 69 1 view .LVU77
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE64:
	.size	_cgo_libc_setresgid, .-_cgo_libc_setresgid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setresuid
	.type	_cgo_libc_setresuid, %function
_cgo_libc_setresuid:
.LVL36:
.LFB65:
	.loc 1 72 34 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 73 2 view .LVU79
	.loc 1 72 34 is_stmt 0 view .LVU80
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	.loc 1 73 2 view .LVU81
	ldr	x2, [x0]
	ldr	w1, [x2, 8]
	.loc 1 72 34 view .LVU82
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 72 34 view .LVU83
	mov	x19, x0
	.loc 1 73 2 view .LVU84
	ldr	w0, [x2]
.LVL37:
	.loc 1 73 2 view .LVU85
	ldr	w2, [x2, 16]
	bl	setresuid
.LVL38:
	.loc 1 73 2 is_stmt 1 view .LVU86
	sxtw	x1, w0
.LVL39:
	.loc 1 73 2 is_stmt 0 view .LVU87
	cmn	w0, #1
	bne	.L27
	.loc 1 73 2 is_stmt 1 discriminator 1 view .LVU88
	bl	__errno_location
.LVL40:
	.loc 1 73 2 is_stmt 0 discriminator 1 view .LVU89
	ldrsw	x1, [x0]
.L27:
	str	x1, [x19, 8]
	.loc 1 75 1 view .LVU90
	ldr	x19, [sp, 16]
.LVL41:
	.loc 1 75 1 view .LVU91
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE65:
	.size	_cgo_libc_setresuid, .-_cgo_libc_setresuid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setreuid
	.type	_cgo_libc_setreuid, %function
_cgo_libc_setreuid:
.LVL42:
.LFB66:
	.loc 1 78 33 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 79 2 view .LVU93
	.loc 1 78 33 is_stmt 0 view .LVU94
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	.loc 1 79 2 view .LVU95
	ldr	x1, [x0]
	.loc 1 78 33 view .LVU96
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 78 33 view .LVU97
	mov	x19, x0
	.loc 1 79 2 view .LVU98
	ldr	w0, [x1]
.LVL43:
	.loc 1 79 2 view .LVU99
	ldr	w1, [x1, 8]
	bl	setreuid
.LVL44:
	.loc 1 79 2 is_stmt 1 view .LVU100
	sxtw	x1, w0
.LVL45:
	.loc 1 79 2 is_stmt 0 view .LVU101
	cmn	w0, #1
	bne	.L31
	.loc 1 79 2 is_stmt 1 discriminator 1 view .LVU102
	bl	__errno_location
.LVL46:
	.loc 1 79 2 is_stmt 0 discriminator 1 view .LVU103
	ldrsw	x1, [x0]
.L31:
	str	x1, [x19, 8]
	.loc 1 80 1 view .LVU104
	ldr	x19, [sp, 16]
.LVL47:
	.loc 1 80 1 view .LVU105
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE66:
	.size	_cgo_libc_setreuid, .-_cgo_libc_setreuid
	.align	2
	.p2align 4,,11
	.global	_cgo_libc_setuid
	.type	_cgo_libc_setuid, %function
_cgo_libc_setuid:
.LVL48:
.LFB67:
	.loc 1 83 31 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 84 2 view .LVU107
	.loc 1 83 31 is_stmt 0 view .LVU108
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	str	x19, [sp, 16]
	.cfi_offset 19, -16
	.loc 1 83 31 view .LVU109
	mov	x19, x0
	.loc 1 84 2 view .LVU110
	ldr	x0, [x0]
.LVL49:
	.loc 1 84 2 view .LVU111
	ldr	w0, [x0]
	bl	setuid
.LVL50:
	.loc 1 84 2 is_stmt 1 view .LVU112
	sxtw	x1, w0
.LVL51:
	.loc 1 84 2 is_stmt 0 view .LVU113
	cmn	w0, #1
	bne	.L35
	.loc 1 84 2 is_stmt 1 discriminator 1 view .LVU114
	bl	__errno_location
.LVL52:
	.loc 1 84 2 is_stmt 0 discriminator 1 view .LVU115
	ldrsw	x1, [x0]
.L35:
	str	x1, [x19, 8]
	.loc 1 85 1 view .LVU116
	ldr	x19, [sp, 16]
.LVL53:
	.loc 1 85 1 view .LVU117
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE67:
	.size	_cgo_libc_setuid, .-_cgo_libc_setuid
.Letext0:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 3 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 4 "/usr/include/grp.h"
	.file 5 "/usr/include/aarch64-linux-gnu/sys/types.h"
	.file 6 "/usr/include/stdint.h"
	.file 7 "/usr/include/unistd.h"
	.file 8 "/usr/include/errno.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x51c
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0xb
	.4byte	.LASF38
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x3
	.byte	0x1
	.byte	0x8
	.4byte	.LASF2
	.uleb128 0x3
	.byte	0x2
	.byte	0x7
	.4byte	.LASF3
	.uleb128 0x3
	.byte	0x4
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x3
	.byte	0x8
	.byte	0x7
	.4byte	.LASF5
	.uleb128 0x3
	.byte	0x1
	.byte	0x6
	.4byte	.LASF6
	.uleb128 0x3
	.byte	0x2
	.byte	0x5
	.4byte	.LASF7
	.uleb128 0xc
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x3
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x8
	.4byte	.LASF9
	.byte	0x2
	.byte	0x92
	.byte	0x19
	.4byte	0x3c
	.uleb128 0x8
	.4byte	.LASF10
	.byte	0x2
	.byte	0x93
	.byte	0x19
	.4byte	0x3c
	.uleb128 0xd
	.4byte	0x72
	.uleb128 0x3
	.byte	0x1
	.byte	0x8
	.4byte	.LASF11
	.uleb128 0x8
	.4byte	.LASF12
	.byte	0x3
	.byte	0xd1
	.byte	0x17
	.4byte	0x43
	.uleb128 0x8
	.4byte	.LASF13
	.byte	0x4
	.byte	0x25
	.byte	0x11
	.4byte	0x72
	.uleb128 0x8
	.4byte	.LASF14
	.byte	0x5
	.byte	0x4f
	.byte	0x11
	.4byte	0x66
	.uleb128 0x3
	.byte	0x8
	.byte	0x7
	.4byte	.LASF15
	.uleb128 0x3
	.byte	0x8
	.byte	0x5
	.4byte	.LASF16
	.uleb128 0x8
	.4byte	.LASF17
	.byte	0x6
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x9
	.4byte	0xbc
	.uleb128 0xe
	.byte	0x10
	.byte	0x1
	.byte	0x1a
	.byte	0x9
	.4byte	0xef
	.uleb128 0xa
	.4byte	.LASF18
	.byte	0x1b
	.byte	0xd
	.4byte	0xc8
	.byte	0
	.uleb128 0xa
	.4byte	.LASF19
	.byte	0x1c
	.byte	0xc
	.4byte	0xbc
	.byte	0x8
	.byte	0
	.uleb128 0x8
	.4byte	.LASF20
	.byte	0x1
	.byte	0x1d
	.byte	0x3
	.4byte	0xcd
	.uleb128 0x6
	.4byte	.LASF21
	.2byte	0x2d2
	.4byte	0x58
	.4byte	0x110
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x6
	.4byte	.LASF22
	.2byte	0x2d7
	.4byte	0x58
	.4byte	0x12a
	.uleb128 0x2
	.4byte	0x66
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x6
	.4byte	.LASF23
	.2byte	0x2fd
	.4byte	0x58
	.4byte	0x149
	.uleb128 0x2
	.4byte	0x66
	.uleb128 0x2
	.4byte	0x66
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x6
	.4byte	.LASF24
	.2byte	0x302
	.4byte	0x58
	.4byte	0x168
	.uleb128 0x2
	.4byte	0x72
	.uleb128 0x2
	.4byte	0x72
	.uleb128 0x2
	.4byte	0x72
	.byte	0
	.uleb128 0x6
	.4byte	.LASF25
	.2byte	0x2e8
	.4byte	0x58
	.4byte	0x182
	.uleb128 0x2
	.4byte	0x72
	.uleb128 0x2
	.4byte	0x72
	.byte	0
	.uleb128 0xf
	.4byte	.LASF26
	.byte	0x4
	.byte	0xb0
	.byte	0xc
	.4byte	0x58
	.4byte	0x19d
	.uleb128 0x2
	.4byte	0x8a
	.uleb128 0x2
	.4byte	0x19d
	.byte	0
	.uleb128 0x9
	.4byte	0x7e
	.uleb128 0x6
	.4byte	.LASF27
	.2byte	0x2e3
	.4byte	0x58
	.4byte	0x1b7
	.uleb128 0x2
	.4byte	0x72
	.byte	0
	.uleb128 0x6
	.4byte	.LASF28
	.2byte	0x2dc
	.4byte	0x58
	.4byte	0x1cc
	.uleb128 0x2
	.4byte	0x66
	.byte	0
	.uleb128 0x10
	.4byte	.LASF39
	.byte	0x8
	.byte	0x25
	.byte	0xd
	.4byte	0x1d8
	.uleb128 0x9
	.4byte	0x58
	.uleb128 0x6
	.4byte	.LASF29
	.2byte	0x2ed
	.4byte	0x58
	.4byte	0x1f2
	.uleb128 0x2
	.4byte	0x72
	.byte	0
	.uleb128 0x7
	.4byte	.LASF30
	.byte	0x53
	.8byte	.LFB67
	.8byte	.LFE67-.LFB67
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x24c
	.uleb128 0x4
	.string	"x"
	.byte	0x53
	.byte	0x1c
	.4byte	0x24c
	.4byte	.LLST16
	.4byte	.LVUS16
	.uleb128 0x5
	.string	"ret"
	.byte	0x54
	.4byte	0xbc
	.4byte	.LLST17
	.4byte	.LVUS17
	.uleb128 0x1
	.8byte	.LVL50
	.4byte	0xfb
	.uleb128 0x1
	.8byte	.LVL52
	.4byte	0x1cc
	.byte	0
	.uleb128 0x9
	.4byte	0xef
	.uleb128 0x7
	.4byte	.LASF31
	.byte	0x4e
	.8byte	.LFB66
	.8byte	.LFE66-.LFB66
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x2ab
	.uleb128 0x4
	.string	"x"
	.byte	0x4e
	.byte	0x1e
	.4byte	0x24c
	.4byte	.LLST14
	.4byte	.LVUS14
	.uleb128 0x5
	.string	"ret"
	.byte	0x4f
	.4byte	0xbc
	.4byte	.LLST15
	.4byte	.LVUS15
	.uleb128 0x1
	.8byte	.LVL44
	.4byte	0x110
	.uleb128 0x1
	.8byte	.LVL46
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF32
	.byte	0x48
	.8byte	.LFB65
	.8byte	.LFE65-.LFB65
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x305
	.uleb128 0x4
	.string	"x"
	.byte	0x48
	.byte	0x1f
	.4byte	0x24c
	.4byte	.LLST12
	.4byte	.LVUS12
	.uleb128 0x5
	.string	"ret"
	.byte	0x49
	.4byte	0xbc
	.4byte	.LLST13
	.4byte	.LVUS13
	.uleb128 0x1
	.8byte	.LVL38
	.4byte	0x12a
	.uleb128 0x1
	.8byte	.LVL40
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF33
	.byte	0x42
	.8byte	.LFB64
	.8byte	.LFE64-.LFB64
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x35f
	.uleb128 0x4
	.string	"x"
	.byte	0x42
	.byte	0x1f
	.4byte	0x24c
	.4byte	.LLST10
	.4byte	.LVUS10
	.uleb128 0x5
	.string	"ret"
	.byte	0x43
	.4byte	0xbc
	.4byte	.LLST11
	.4byte	.LVUS11
	.uleb128 0x1
	.8byte	.LVL32
	.4byte	0x149
	.uleb128 0x1
	.8byte	.LVL34
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF34
	.byte	0x3d
	.8byte	.LFB63
	.8byte	.LFE63-.LFB63
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x3b9
	.uleb128 0x4
	.string	"x"
	.byte	0x3d
	.byte	0x1e
	.4byte	0x24c
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x5
	.string	"ret"
	.byte	0x3e
	.4byte	0xbc
	.4byte	.LLST9
	.4byte	.LVUS9
	.uleb128 0x1
	.8byte	.LVL26
	.4byte	0x168
	.uleb128 0x1
	.8byte	.LVL28
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF35
	.byte	0x38
	.8byte	.LFB62
	.8byte	.LFE62-.LFB62
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x413
	.uleb128 0x4
	.string	"x"
	.byte	0x38
	.byte	0x1f
	.4byte	0x24c
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x5
	.string	"ret"
	.byte	0x39
	.4byte	0xbc
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x1
	.8byte	.LVL20
	.4byte	0x182
	.uleb128 0x1
	.8byte	.LVL22
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF36
	.byte	0x33
	.8byte	.LFB61
	.8byte	.LFE61-.LFB61
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x46d
	.uleb128 0x4
	.string	"x"
	.byte	0x33
	.byte	0x1c
	.4byte	0x24c
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x5
	.string	"ret"
	.byte	0x34
	.4byte	0xbc
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0x1
	.8byte	.LVL14
	.4byte	0x1a2
	.uleb128 0x1
	.8byte	.LVL16
	.4byte	0x1cc
	.byte	0
	.uleb128 0x7
	.4byte	.LASF37
	.byte	0x2e
	.8byte	.LFB60
	.8byte	.LFE60-.LFB60
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x4c7
	.uleb128 0x4
	.string	"x"
	.byte	0x2e
	.byte	0x1d
	.4byte	0x24c
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x5
	.string	"ret"
	.byte	0x2f
	.4byte	0xbc
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x1
	.8byte	.LVL8
	.4byte	0x1b7
	.uleb128 0x1
	.8byte	.LVL10
	.4byte	0x1cc
	.byte	0
	.uleb128 0x11
	.4byte	.LASF40
	.byte	0x1
	.byte	0x29
	.byte	0x1
	.8byte	.LFB59
	.8byte	.LFE59-.LFB59
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x4
	.string	"x"
	.byte	0x29
	.byte	0x1d
	.4byte	0x24c
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x5
	.string	"ret"
	.byte	0x2a
	.4byte	0xbc
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x1
	.8byte	.LVL2
	.4byte	0x1dd
	.uleb128 0x1
	.8byte	.LVL4
	.4byte	0x1cc
	.byte	0
	.byte	0
	.section	.debug_abbrev,"",@progbits
.Ldebug_abbrev0:
	.uleb128 0x1
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
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
	.uleb128 0x4
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
	.uleb128 0x5
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
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
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
	.sleb128 7
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
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
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
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xe
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
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x10
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
	.uleb128 0x11
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
	.byte	0
	.section	.debug_loclists,"",@progbits
	.4byte	.Ldebug_loc3-.Ldebug_loc2
.Ldebug_loc2:
	.2byte	0x5
	.byte	0x8
	.byte	0
	.4byte	0
.Ldebug_loc0:
.LVUS16:
	.uleb128 0
	.uleb128 .LVU111
	.uleb128 .LVU111
	.uleb128 .LVU117
	.uleb128 .LVU117
	.uleb128 0
.LLST16:
	.byte	0x4
	.uleb128 .LVL48-.Ltext0
	.uleb128 .LVL49-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL49-.Ltext0
	.uleb128 .LVL53-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL53-.Ltext0
	.uleb128 .LFE67-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS17:
	.uleb128 .LVU112
	.uleb128 .LVU113
	.uleb128 .LVU113
	.uleb128 .LVU115
.LLST17:
	.byte	0x4
	.uleb128 .LVL50-.Ltext0
	.uleb128 .LVL51-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL51-.Ltext0
	.uleb128 .LVL52-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS14:
	.uleb128 0
	.uleb128 .LVU99
	.uleb128 .LVU99
	.uleb128 .LVU105
	.uleb128 .LVU105
	.uleb128 0
.LLST14:
	.byte	0x4
	.uleb128 .LVL42-.Ltext0
	.uleb128 .LVL43-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL43-.Ltext0
	.uleb128 .LVL47-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL47-.Ltext0
	.uleb128 .LFE66-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS15:
	.uleb128 .LVU100
	.uleb128 .LVU101
	.uleb128 .LVU101
	.uleb128 .LVU103
.LLST15:
	.byte	0x4
	.uleb128 .LVL44-.Ltext0
	.uleb128 .LVL45-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL45-.Ltext0
	.uleb128 .LVL46-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS12:
	.uleb128 0
	.uleb128 .LVU85
	.uleb128 .LVU85
	.uleb128 .LVU91
	.uleb128 .LVU91
	.uleb128 0
.LLST12:
	.byte	0x4
	.uleb128 .LVL36-.Ltext0
	.uleb128 .LVL37-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL37-.Ltext0
	.uleb128 .LVL41-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL41-.Ltext0
	.uleb128 .LFE65-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS13:
	.uleb128 .LVU86
	.uleb128 .LVU87
	.uleb128 .LVU87
	.uleb128 .LVU89
.LLST13:
	.byte	0x4
	.uleb128 .LVL38-.Ltext0
	.uleb128 .LVL39-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL39-.Ltext0
	.uleb128 .LVL40-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS10:
	.uleb128 0
	.uleb128 .LVU71
	.uleb128 .LVU71
	.uleb128 .LVU77
	.uleb128 .LVU77
	.uleb128 0
.LLST10:
	.byte	0x4
	.uleb128 .LVL30-.Ltext0
	.uleb128 .LVL31-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL31-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LFE64-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS11:
	.uleb128 .LVU72
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU75
.LLST11:
	.byte	0x4
	.uleb128 .LVL32-.Ltext0
	.uleb128 .LVL33-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL33-.Ltext0
	.uleb128 .LVL34-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS8:
	.uleb128 0
	.uleb128 .LVU57
	.uleb128 .LVU57
	.uleb128 .LVU63
	.uleb128 .LVU63
	.uleb128 0
.LLST8:
	.byte	0x4
	.uleb128 .LVL24-.Ltext0
	.uleb128 .LVL25-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL25-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LFE63-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS9:
	.uleb128 .LVU58
	.uleb128 .LVU59
	.uleb128 .LVU59
	.uleb128 .LVU61
.LLST9:
	.byte	0x4
	.uleb128 .LVL26-.Ltext0
	.uleb128 .LVL27-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL27-.Ltext0
	.uleb128 .LVL28-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS6:
	.uleb128 0
	.uleb128 .LVU43
	.uleb128 .LVU43
	.uleb128 .LVU49
	.uleb128 .LVU49
	.uleb128 0
.LLST6:
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LVL19-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL19-.Ltext0
	.uleb128 .LVL23-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL23-.Ltext0
	.uleb128 .LFE62-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS7:
	.uleb128 .LVU44
	.uleb128 .LVU45
	.uleb128 .LVU45
	.uleb128 .LVU47
.LLST7:
	.byte	0x4
	.uleb128 .LVL20-.Ltext0
	.uleb128 .LVL21-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL22-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS4:
	.uleb128 0
	.uleb128 .LVU29
	.uleb128 .LVU29
	.uleb128 .LVU35
	.uleb128 .LVU35
	.uleb128 0
.LLST4:
	.byte	0x4
	.uleb128 .LVL12-.Ltext0
	.uleb128 .LVL13-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL13-.Ltext0
	.uleb128 .LVL17-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL17-.Ltext0
	.uleb128 .LFE61-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS5:
	.uleb128 .LVU30
	.uleb128 .LVU31
	.uleb128 .LVU31
	.uleb128 .LVU33
.LLST5:
	.byte	0x4
	.uleb128 .LVL14-.Ltext0
	.uleb128 .LVL15-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU17
	.uleb128 .LVU17
	.uleb128 .LVU23
	.uleb128 .LVU23
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL6-.Ltext0
	.uleb128 .LVL7-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LVL11-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL11-.Ltext0
	.uleb128 .LFE60-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS3:
	.uleb128 .LVU18
	.uleb128 .LVU19
	.uleb128 .LVU19
	.uleb128 .LVU21
.LLST3:
	.byte	0x4
	.uleb128 .LVL8-.Ltext0
	.uleb128 .LVL9-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL9-.Ltext0
	.uleb128 .LVL10-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0
.LVUS0:
	.uleb128 0
	.uleb128 .LVU5
	.uleb128 .LVU5
	.uleb128 .LVU11
	.uleb128 .LVU11
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL5-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL5-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU6
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU9
.LLST1:
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LVL3-.Ltext0
	.uleb128 0x9
	.byte	0x70
	.sleb128 0
	.byte	0x8
	.byte	0x20
	.byte	0x24
	.byte	0x8
	.byte	0x20
	.byte	0x26
	.byte	0x9f
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
.LASF13:
	.string	"gid_t"
.LASF12:
	.string	"size_t"
.LASF28:
	.string	"seteuid"
.LASF26:
	.string	"setgroups"
.LASF9:
	.string	"__uid_t"
.LASF35:
	.string	"_cgo_libc_setgroups"
.LASF23:
	.string	"setresuid"
.LASF31:
	.string	"_cgo_libc_setreuid"
.LASF6:
	.string	"signed char"
.LASF14:
	.string	"uid_t"
.LASF2:
	.string	"unsigned char"
.LASF19:
	.string	"retval"
.LASF24:
	.string	"setresgid"
.LASF22:
	.string	"setreuid"
.LASF5:
	.string	"long unsigned int"
.LASF3:
	.string	"short unsigned int"
.LASF17:
	.string	"uintptr_t"
.LASF20:
	.string	"argset_t"
.LASF37:
	.string	"_cgo_libc_seteuid"
.LASF21:
	.string	"setuid"
.LASF25:
	.string	"setregid"
.LASF30:
	.string	"_cgo_libc_setuid"
.LASF32:
	.string	"_cgo_libc_setresuid"
.LASF4:
	.string	"unsigned int"
.LASF15:
	.string	"long long unsigned int"
.LASF40:
	.string	"_cgo_libc_setegid"
.LASF27:
	.string	"setgid"
.LASF36:
	.string	"_cgo_libc_setgid"
.LASF10:
	.string	"__gid_t"
.LASF33:
	.string	"_cgo_libc_setresgid"
.LASF16:
	.string	"long long int"
.LASF11:
	.string	"char"
.LASF38:
	.string	"GNU C17 11.4.0"
.LASF7:
	.string	"short int"
.LASF18:
	.string	"args"
.LASF29:
	.string	"setegid"
.LASF8:
	.string	"long int"
.LASF39:
	.string	"__errno_location"
.LASF34:
	.string	"_cgo_libc_setregid"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"linux_syscall.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
