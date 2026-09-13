	.arch armv8-a
	.file	"gcc_mmap.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_mmap.c"
	.align	2
	.p2align 4,,11
	.global	x_cgo_mmap
	.type	x_cgo_mmap, %function
x_cgo_mmap:
.LVL0:
.LFB39:
	.file 1 "gcc_mmap.c"
	.loc 1 15 100 view -0
	.cfi_startproc
	.loc 1 16 2 view .LVU1
	.loc 1 18 21 view .LVU2
	.loc 1 19 2 view .LVU3
	.loc 1 15 100 is_stmt 0 view .LVU4
	stp	x29, x30, [sp, -16]!
	.cfi_def_cfa_offset 16
	.cfi_offset 29, -16
	.cfi_offset 30, -8
	.loc 1 19 6 view .LVU5
	uxtw	x5, w5
	.loc 1 15 100 view .LVU6
	mov	x29, sp
	.loc 1 19 6 view .LVU7
	bl	mmap
.LVL1:
	.loc 1 20 21 is_stmt 1 view .LVU8
	.loc 1 21 2 view .LVU9
	.loc 1 21 5 is_stmt 0 view .LVU10
	cmn	x0, #1
	beq	.L6
	.loc 1 26 1 view .LVU11
	ldp	x29, x30, [sp], 16
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_def_cfa_offset 0
	ret
	.p2align 2,,3
.L6:
	.cfi_restore_state
	.loc 1 23 3 is_stmt 1 view .LVU12
	.loc 1 23 21 is_stmt 0 view .LVU13
	bl	__errno_location
.LVL2:
	.loc 1 23 10 view .LVU14
	ldrsw	x0, [x0]
	.loc 1 26 1 view .LVU15
	ldp	x29, x30, [sp], 16
	.cfi_restore 30
	.cfi_restore 29
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE39:
	.size	x_cgo_mmap, .-x_cgo_mmap
	.align	2
	.p2align 4,,11
	.global	x_cgo_munmap
	.type	x_cgo_munmap, %function
x_cgo_munmap:
.LVL3:
.LFB40:
	.loc 1 29 44 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 30 2 view .LVU17
	.loc 1 32 21 view .LVU18
	.loc 1 33 2 view .LVU19
	.loc 1 29 44 is_stmt 0 view .LVU20
	stp	x29, x30, [sp, -16]!
	.cfi_def_cfa_offset 16
	.cfi_offset 29, -16
	.cfi_offset 30, -8
	mov	x29, sp
	.loc 1 33 6 view .LVU21
	bl	munmap
.LVL4:
	.loc 1 34 21 is_stmt 1 view .LVU22
	.loc 1 35 2 view .LVU23
	.loc 1 35 5 is_stmt 0 view .LVU24
	tbnz	w0, #31, .L10
	.loc 1 39 1 view .LVU25
	ldp	x29, x30, [sp], 16
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_def_cfa_offset 0
	ret
.L10:
	.cfi_restore_state
	.loc 1 37 3 is_stmt 1 view .LVU26
	bl	abort
.LVL5:
	.loc 1 37 3 is_stmt 0 view .LVU27
	.cfi_endproc
.LFE40:
	.size	x_cgo_munmap, .-x_cgo_munmap
.Letext0:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 3 "/usr/include/aarch64-linux-gnu/bits/stdint-intn.h"
	.file 4 "/usr/include/aarch64-linux-gnu/bits/stdint-uintn.h"
	.file 5 "/usr/include/stdint.h"
	.file 6 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 7 "/usr/include/stdlib.h"
	.file 8 "/usr/include/errno.h"
	.file 9 "/usr/include/aarch64-linux-gnu/sys/mman.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0x2a5
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0xa
	.4byte	.LASF30
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
	.4byte	.LASF8
	.byte	0x2
	.byte	0x29
	.byte	0x14
	.4byte	0x64
	.uleb128 0xb
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x4
	.4byte	.LASF9
	.byte	0x2
	.byte	0x2a
	.byte	0x16
	.4byte	0x3c
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF10
	.uleb128 0x4
	.4byte	.LASF11
	.byte	0x2
	.byte	0x98
	.byte	0x19
	.4byte	0x77
	.uleb128 0xc
	.byte	0x8
	.uleb128 0x1
	.byte	0x1
	.byte	0x8
	.4byte	.LASF12
	.uleb128 0x4
	.4byte	.LASF13
	.byte	0x3
	.byte	0x1a
	.byte	0x13
	.4byte	0x58
	.uleb128 0x4
	.4byte	.LASF14
	.byte	0x4
	.byte	0x1a
	.byte	0x14
	.4byte	0x6b
	.uleb128 0x4
	.4byte	.LASF15
	.byte	0x5
	.byte	0x5a
	.byte	0x1b
	.4byte	0x43
	.uleb128 0x4
	.4byte	.LASF16
	.byte	0x6
	.byte	0xd1
	.byte	0x17
	.4byte	0x43
	.uleb128 0x1
	.byte	0x8
	.byte	0x5
	.4byte	.LASF17
	.uleb128 0x1
	.byte	0x8
	.byte	0x7
	.4byte	.LASF18
	.uleb128 0xd
	.4byte	.LASF19
	.byte	0x7
	.2byte	0x256
	.byte	0xd
	.uleb128 0x6
	.4byte	.LASF21
	.byte	0x4c
	.byte	0xc
	.4byte	0x64
	.4byte	0xf4
	.uleb128 0x2
	.4byte	0x8a
	.uleb128 0x2
	.4byte	0xb7
	.byte	0
	.uleb128 0xe
	.4byte	.LASF20
	.byte	0x8
	.byte	0x25
	.byte	0xd
	.4byte	0x100
	.uleb128 0xf
	.byte	0x8
	.4byte	0x64
	.uleb128 0x6
	.4byte	.LASF22
	.byte	0x39
	.byte	0xe
	.4byte	0x8a
	.4byte	0x134
	.uleb128 0x2
	.4byte	0x8a
	.uleb128 0x2
	.4byte	0xb7
	.uleb128 0x2
	.4byte	0x64
	.uleb128 0x2
	.4byte	0x64
	.uleb128 0x2
	.4byte	0x64
	.uleb128 0x2
	.4byte	0x7e
	.byte	0
	.uleb128 0x10
	.4byte	.LASF25
	.byte	0x1
	.byte	0x1d
	.byte	0x1
	.8byte	.LFB40
	.8byte	.LFE40-.LFB40
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x1b7
	.uleb128 0x5
	.4byte	.LASF23
	.byte	0x1d
	.byte	0x14
	.4byte	0x8a
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0x5
	.4byte	.LASF24
	.byte	0x1d
	.byte	0x24
	.4byte	0xab
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x7
	.string	"r"
	.byte	0x1e
	.byte	0x6
	.4byte	0x64
	.4byte	.LLST9
	.4byte	.LVUS9
	.uleb128 0x8
	.8byte	.LVL4
	.4byte	0xda
	.4byte	0x1a9
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0
	.uleb128 0x9
	.8byte	.LVL5
	.4byte	0xd1
	.byte	0
	.uleb128 0x11
	.4byte	.LASF26
	.byte	0x1
	.byte	0xf
	.byte	0x1
	.4byte	0xab
	.8byte	.LFB39
	.8byte	.LFE39-.LFB39
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x5
	.4byte	.LASF23
	.byte	0xf
	.byte	0x12
	.4byte	0x8a
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x5
	.4byte	.LASF24
	.byte	0xf
	.byte	0x22
	.4byte	0xab
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x5
	.4byte	.LASF27
	.byte	0xf
	.byte	0x32
	.4byte	0x93
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x5
	.4byte	.LASF28
	.byte	0xf
	.byte	0x40
	.4byte	0x93
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0x12
	.string	"fd"
	.byte	0x1
	.byte	0xf
	.byte	0x4f
	.4byte	0x93
	.4byte	.LLST4
	.4byte	.LVUS4
	.uleb128 0x5
	.4byte	.LASF29
	.byte	0xf
	.byte	0x5c
	.4byte	0x9f
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0x7
	.string	"p"
	.byte	0x10
	.byte	0x8
	.4byte	0x8a
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x8
	.8byte	.LVL1
	.4byte	0x106
	.4byte	0x29a
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x54
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x54
	.uleb128 0x3
	.uleb128 0x1
	.byte	0x55
	.uleb128 0x9
	.byte	0xa3
	.uleb128 0x1
	.byte	0x55
	.byte	0xc
	.4byte	0xffffffff
	.byte	0x1a
	.byte	0
	.uleb128 0x9
	.8byte	.LVL2
	.4byte	0xf4
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
	.sleb128 9
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
	.uleb128 0x7
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
	.uleb128 0x8
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
	.uleb128 0x9
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xa
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
	.uleb128 0xb
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
	.uleb128 0xc
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0xd
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
	.uleb128 0x87
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0xe
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
	.uleb128 0xf
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
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
	.byte	0
	.byte	0
	.uleb128 0x12
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
	.byte	0
	.section	.debug_loclists,"",@progbits
	.4byte	.Ldebug_loc3-.Ldebug_loc2
.Ldebug_loc2:
	.2byte	0x5
	.byte	0x8
	.byte	0
	.4byte	0
.Ldebug_loc0:
.LVUS7:
	.uleb128 0
	.uleb128 .LVU22
	.uleb128 .LVU22
	.uleb128 0
.LLST7:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 .LFE40-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS8:
	.uleb128 0
	.uleb128 .LVU22
	.uleb128 .LVU22
	.uleb128 0
.LLST8:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL4-1-.Ltext0
	.uleb128 .LFE40-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS9:
	.uleb128 .LVU22
	.uleb128 .LVU27
.LLST9:
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL5-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS0:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST1:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x52
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0
.LVUS3:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST3:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x53
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0x9f
	.byte	0
.LVUS4:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST4:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x54
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x54
	.byte	0x9f
	.byte	0
.LVUS5:
	.uleb128 0
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST5:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 0x1
	.byte	0x55
	.byte	0x4
	.uleb128 .LVL1-1-.Ltext0
	.uleb128 .LFE39-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x55
	.byte	0x9f
	.byte	0
.LVUS6:
	.uleb128 .LVU8
	.uleb128 .LVU14
.LLST6:
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-1-.Ltext0
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
.LASF16:
	.string	"size_t"
.LASF15:
	.string	"uintptr_t"
.LASF8:
	.string	"__int32_t"
.LASF3:
	.string	"short unsigned int"
.LASF26:
	.string	"x_cgo_mmap"
.LASF27:
	.string	"prot"
.LASF2:
	.string	"unsigned char"
.LASF21:
	.string	"munmap"
.LASF5:
	.string	"long unsigned int"
.LASF23:
	.string	"addr"
.LASF22:
	.string	"mmap"
.LASF9:
	.string	"__uint32_t"
.LASF4:
	.string	"unsigned int"
.LASF28:
	.string	"flags"
.LASF18:
	.string	"long long unsigned int"
.LASF20:
	.string	"__errno_location"
.LASF13:
	.string	"int32_t"
.LASF11:
	.string	"__off_t"
.LASF17:
	.string	"long long int"
.LASF12:
	.string	"char"
.LASF30:
	.string	"GNU C17 11.4.0"
.LASF29:
	.string	"offset"
.LASF7:
	.string	"short int"
.LASF25:
	.string	"x_cgo_munmap"
.LASF14:
	.string	"uint32_t"
.LASF10:
	.string	"long int"
.LASF19:
	.string	"abort"
.LASF6:
	.string	"signed char"
.LASF24:
	.string	"length"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_mmap.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
