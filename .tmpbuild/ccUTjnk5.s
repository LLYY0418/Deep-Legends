	.arch armv8-a
	.file	"gcc_libinit.c"
	.text
.Ltext0:
	.file 0 "/_/GOROOT/src/runtime/cgo" "gcc_libinit.c"
	.align	2
	.p2align 4,,11
	.type	pthread_key_destructor, %function
pthread_key_destructor:
.LVL0:
.LFB60:
	.file 1 "gcc_libinit.c"
	.loc 1 173 33 view -0
	.cfi_startproc
	.loc 1 174 2 view .LVU1
	.loc 1 174 23 is_stmt 0 view .LVU2
	adrp	x2, :got:x_crosscall2_ptr
	.loc 1 173 33 view .LVU3
	mov	x1, x0
	.loc 1 174 23 view .LVU4
	ldr	x2, [x2, #:got_lo12:x_crosscall2_ptr]
	ldr	x4, [x2]
	.loc 1 174 5 view .LVU5
	cbz	x4, .L1
	.loc 1 179 3 is_stmt 1 view .LVU6
	mov	x16, x4
	mov	x3, 0
	mov	w2, 0
	mov	x0, 0
.LVL1:
	.loc 1 179 3 is_stmt 0 view .LVU7
	br	x16
.LVL2:
	.p2align 2,,3
.L1:
	.loc 1 181 1 view .LVU8
	ret
	.cfi_endproc
.LFE60:
	.size	pthread_key_destructor, .-pthread_key_destructor
	.align	2
	.p2align 4,,11
	.global	_cgo_wait_runtime_init_done
	.type	_cgo_wait_runtime_init_done, %function
_cgo_wait_runtime_init_done:
.LFB53:
	.loc 1 53 35 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 54 2 view .LVU10
	.loc 1 55 2 view .LVU11
	.loc 1 53 35 is_stmt 0 view .LVU12
	stp	x29, x30, [sp, -80]!
	.cfi_def_cfa_offset 80
	.cfi_offset 29, -80
	.cfi_offset 30, -72
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	stp	x21, x22, [sp, 32]
	.cfi_offset 19, -64
	.cfi_offset 20, -56
	.cfi_offset 21, -48
	.cfi_offset 22, -40
	.loc 1 55 8 view .LVU13
	adrp	x21, .LANCHOR0
	.loc 1 53 35 view .LVU14
	str	x23, [sp, 48]
	.cfi_offset 23, -32
	.loc 1 55 8 view .LVU15
	add	x23, x21, :lo12:.LANCHOR0
	ldar	x0, [x23]
.LVL3:
	.loc 1 57 2 is_stmt 1 view .LVU16
	.loc 1 58 2 view .LVU17
	.loc 1 58 6 is_stmt 0 view .LVU18
	add	x19, x23, 8
	ldar	w1, [x19]
	.loc 1 58 5 view .LVU19
	cmp	w1, 2
	bne	.L5
	.loc 1 55 6 view .LVU20
	mov	x19, x0
.LVL4:
.L6:
	.loc 1 86 2 is_stmt 1 view .LVU21
	.loc 1 93 9 is_stmt 0 view .LVU22
	mov	x0, 0
	.loc 1 86 5 view .LVU23
	cbz	x19, .L4
.LBB6:
	.loc 1 87 3 is_stmt 1 view .LVU24
	.loc 1 89 3 view .LVU25
	.loc 1 89 15 is_stmt 0 view .LVU26
	str	xzr, [sp, 72]
	.loc 1 90 3 is_stmt 1 view .LVU27
	.loc 1 90 4 is_stmt 0 view .LVU28
	add	x0, sp, 72
	blr	x19
.LVL5:
	.loc 1 91 3 is_stmt 1 view .LVU29
	.loc 1 91 13 is_stmt 0 view .LVU30
	ldr	x0, [sp, 72]
.L4:
.LBE6:
	.loc 1 94 1 view .LVU31
	ldp	x19, x20, [sp, 16]
.LVL6:
	.loc 1 94 1 view .LVU32
	ldp	x21, x22, [sp, 32]
	ldr	x23, [sp, 48]
	ldp	x29, x30, [sp], 80
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 23
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL7:
	.p2align 2,,3
.L5:
	.cfi_restore_state
	.loc 1 59 3 is_stmt 1 view .LVU33
	add	x20, x23, 16
	.loc 1 61 4 is_stmt 0 view .LVU34
	add	x22, x23, 64
	.loc 1 59 3 view .LVU35
	mov	x0, x20
.LVL8:
	.loc 1 59 3 view .LVU36
	bl	pthread_mutex_lock
.LVL9:
	.loc 1 60 3 is_stmt 1 view .LVU37
	.loc 1 60 9 is_stmt 0 view .LVU38
	b	.L7
	.p2align 2,,3
.L8:
	.loc 1 61 4 view .LVU39
	mov	x1, x20
	mov	x0, x22
	bl	pthread_cond_wait
.LVL10:
.L7:
	.loc 1 60 64 is_stmt 1 view .LVU40
	.loc 1 60 10 is_stmt 0 view .LVU41
	ldar	w0, [x19]
	.loc 1 61 4 is_stmt 1 view .LVU42
	.loc 1 60 64 is_stmt 0 view .LVU43
	cbz	w0, .L8
	.loc 1 66 3 is_stmt 1 view .LVU44
	.loc 1 66 33 is_stmt 0 view .LVU45
	adrp	x19, :got:x_cgo_pthread_key_created
	ldr	x19, [x19, #:got_lo12:x_cgo_pthread_key_created]
	.loc 1 66 6 view .LVU46
	ldr	x0, [x19]
	cbz	x0, .L16
.L10:
	.loc 1 80 3 is_stmt 1 view .LVU47
	.loc 1 80 9 is_stmt 0 view .LVU48
	add	x0, x21, :lo12:.LANCHOR0
	ldar	x19, [x0]
.LVL11:
	.loc 1 82 3 is_stmt 1 view .LVU49
	mov	w2, 2
	add	x1, x0, 8
	stlr	w2, [x1]
	.loc 1 83 3 view .LVU50
	add	x0, x0, 16
	bl	pthread_mutex_unlock
.LVL12:
	b	.L6
.LVL13:
	.p2align 2,,3
.L16:
	.loc 1 66 41 is_stmt 0 discriminator 1 view .LVU51
	adrp	x1, pthread_key_destructor
	add	x0, x23, 112
	add	x1, x1, :lo12:pthread_key_destructor
	bl	pthread_key_create
.LVL14:
	.loc 1 66 38 discriminator 1 view .LVU52
	cbnz	w0, .L10
	.loc 1 67 4 is_stmt 1 view .LVU53
	.loc 1 67 30 is_stmt 0 view .LVU54
	mov	x0, 1
	str	x0, [x19]
	b	.L10
	.cfi_endproc
.LFE53:
	.size	_cgo_wait_runtime_init_done, .-_cgo_wait_runtime_init_done
	.section	.rodata.str1.8,"aMS",@progbits,1
	.align	3
.LC0:
	.string	"runtime/cgo: bad stack bounds: lo=%p hi=%p\n"
	.text
	.align	2
	.p2align 4,,11
	.global	_cgo_set_stacklo
	.type	_cgo_set_stacklo, %function
_cgo_set_stacklo:
.LVL15:
.LFB54:
	.loc 1 100 1 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 101 2 view .LVU56
	.loc 1 104 2 view .LVU57
	.loc 1 100 1 is_stmt 0 view .LVU58
	stp	x29, x30, [sp, -48]!
	.cfi_def_cfa_offset 48
	.cfi_offset 29, -48
	.cfi_offset 30, -40
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -32
	.cfi_offset 20, -24
	.loc 1 100 1 view .LVU59
	mov	x19, x1
	.loc 1 105 11 view .LVU60
	cmp	x19, 0
	add	x1, sp, 32
.LVL16:
	.loc 1 100 1 view .LVU61
	mov	x20, x0
	.loc 1 105 11 view .LVU62
	csel	x19, x1, x19, eq
.LVL17:
	.loc 1 108 2 is_stmt 1 view .LVU63
	mov	x0, x19
.LVL18:
	.loc 1 108 2 is_stmt 0 view .LVU64
	bl	x_cgo_getstackbound
.LVL19:
	.loc 1 110 2 is_stmt 1 view .LVU65
	.loc 1 110 15 is_stmt 0 view .LVU66
	ldr	x3, [x19]
	.loc 1 110 13 view .LVU67
	str	x3, [x20]
	.loc 1 114 2 is_stmt 1 view .LVU68
	.loc 1 114 21 is_stmt 0 view .LVU69
	ldr	x4, [x20, 8]
	.loc 1 114 5 view .LVU70
	cmp	x3, x4
	bcs	.L21
	.loc 1 118 1 view .LVU71
	ldp	x19, x20, [sp, 16]
.LVL20:
	.loc 1 118 1 view .LVU72
	ldp	x29, x30, [sp], 48
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
.LVL21:
	.loc 1 118 1 view .LVU73
	ret
.LVL22:
.L21:
	.cfi_restore_state
	.loc 1 115 3 is_stmt 1 view .LVU74
.LBB7:
.LBI7:
	.file 2 "/usr/include/aarch64-linux-gnu/bits/stdio2.h"
	.loc 2 103 1 view .LVU75
.LBB8:
	.loc 2 105 3 view .LVU76
.LBE8:
.LBE7:
	.loc 1 115 3 is_stmt 0 view .LVU77
	adrp	x0, :got:stderr
.LBB11:
.LBB9:
	.loc 2 105 10 view .LVU78
	adrp	x2, .LC0
	add	x2, x2, :lo12:.LC0
	mov	w1, 1
.LBE9:
.LBE11:
	.loc 1 115 3 view .LVU79
	ldr	x0, [x0, #:got_lo12:stderr]
.LBB12:
.LBB10:
	.loc 2 105 10 view .LVU80
	ldr	x0, [x0]
	bl	__fprintf_chk
.LVL23:
	.loc 2 105 10 view .LVU81
.LBE10:
.LBE12:
	.loc 1 116 3 is_stmt 1 view .LVU82
	bl	abort
.LVL24:
	.cfi_endproc
.LFE54:
	.size	_cgo_set_stacklo, .-_cgo_set_stacklo
	.align	2
	.p2align 4,,11
	.global	x_cgo_bindm
	.type	x_cgo_bindm, %function
x_cgo_bindm:
.LVL25:
.LFB55:
	.loc 1 122 27 view -0
	.cfi_startproc
	.loc 1 127 2 view .LVU84
	adrp	x2, .LANCHOR0+112
	mov	x1, x0
	ldr	w0, [x2, #:lo12:.LANCHOR0+112]
.LVL26:
	.loc 1 127 2 is_stmt 0 view .LVU85
	b	pthread_setspecific
.LVL27:
	.loc 1 127 2 view .LVU86
	.cfi_endproc
.LFE55:
	.size	x_cgo_bindm, .-x_cgo_bindm
	.align	2
	.p2align 4,,11
	.global	x_cgo_notify_runtime_init_done
	.type	x_cgo_notify_runtime_init_done, %function
x_cgo_notify_runtime_init_done:
.LVL28:
.LFB56:
	.loc 1 131 70 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 132 2 view .LVU88
	.loc 1 131 70 is_stmt 0 view .LVU89
	stp	x29, x30, [sp, -32]!
	.cfi_def_cfa_offset 32
	.cfi_offset 29, -32
	.cfi_offset 30, -24
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -16
	.cfi_offset 20, -8
	.loc 1 132 2 view .LVU90
	adrp	x19, .LANCHOR0
	add	x19, x19, :lo12:.LANCHOR0
	add	x20, x19, 16
	mov	x0, x20
.LVL29:
	.loc 1 132 2 view .LVU91
	bl	pthread_mutex_lock
.LVL30:
	.loc 1 133 2 is_stmt 1 view .LVU92
	mov	w1, 1
	add	x0, x19, 8
	stlr	w1, [x0]
	.loc 1 134 2 view .LVU93
	add	x0, x19, 64
	bl	pthread_cond_broadcast
.LVL31:
	.loc 1 135 2 view .LVU94
	mov	x0, x20
	.loc 1 136 1 is_stmt 0 view .LVU95
	ldp	x19, x20, [sp, 16]
	ldp	x29, x30, [sp], 32
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	.loc 1 135 2 view .LVU96
	b	pthread_mutex_unlock
.LVL32:
	.cfi_endproc
.LFE56:
	.size	x_cgo_notify_runtime_init_done, .-x_cgo_notify_runtime_init_done
	.align	2
	.p2align 4,,11
	.global	x_cgo_set_context_function
	.type	x_cgo_set_context_function, %function
x_cgo_set_context_function:
.LVL33:
.LFB57:
	.loc 1 140 71 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 141 2 view .LVU98
	adrp	x1, .LANCHOR0
	add	x1, x1, :lo12:.LANCHOR0
	stlr	x0, [x1]
	.loc 1 142 1 is_stmt 0 view .LVU99
	ret
	.cfi_endproc
.LFE57:
	.size	x_cgo_set_context_function, .-x_cgo_set_context_function
	.align	2
	.p2align 4,,11
	.global	_cgo_get_context_function
	.type	_cgo_get_context_function, %function
_cgo_get_context_function:
.LFB58:
	.loc 1 145 64 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 146 2 view .LVU101
	.loc 1 146 9 is_stmt 0 view .LVU102
	adrp	x0, .LANCHOR0
	add	x0, x0, :lo12:.LANCHOR0
	ldar	x0, [x0]
	.loc 1 147 1 view .LVU103
	ret
	.cfi_endproc
.LFE58:
	.size	_cgo_get_context_function, .-_cgo_get_context_function
	.align	2
	.p2align 4,,11
	.global	_cgo_try_pthread_create
	.type	_cgo_try_pthread_create, %function
_cgo_try_pthread_create:
.LVL34:
.LFB59:
	.loc 1 152 104 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 153 2 view .LVU105
	.loc 1 154 2 view .LVU106
	.loc 1 155 2 view .LVU107
	.loc 1 157 2 view .LVU108
	.loc 1 157 24 view .LVU109
	.loc 1 152 104 is_stmt 0 view .LVU110
	stp	x29, x30, [sp, -112]!
	.cfi_def_cfa_offset 112
	.cfi_offset 29, -112
	.cfi_offset 30, -104
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -96
	.cfi_offset 20, -88
	mov	x20, 16960
	movk	x20, 0xf, lsl 16
	stp	x21, x22, [sp, 32]
	.cfi_offset 21, -80
	.cfi_offset 22, -72
	mov	x22, x2
	mov	x21, x3
	stp	x23, x24, [sp, 48]
	.cfi_offset 23, -64
	.cfi_offset 24, -56
	mov	x24, x0
	mov	x23, x1
	stp	x25, x26, [sp, 64]
	.cfi_offset 25, -48
	.cfi_offset 26, -40
	.loc 1 157 24 view .LVU111
	mov	x26, 28480
	.loc 1 167 3 view .LVU112
	add	x25, sp, 96
	.loc 1 152 104 view .LVU113
	str	x27, [sp, 80]
	.cfi_offset 27, -32
	.loc 1 157 24 view .LVU114
	mov	x27, x20
	movk	x26, 0x140, lsl 16
.LVL35:
	.p2align 3,,7
.L29:
	.loc 1 158 3 is_stmt 1 view .LVU115
	.loc 1 158 9 is_stmt 0 view .LVU116
	mov	x1, x23
	mov	x3, x21
	mov	x2, x22
	mov	x0, x24
	bl	pthread_create
.LVL36:
	.loc 1 162 3 is_stmt 1 view .LVU117
	.loc 1 165 3 view .LVU118
	.loc 1 158 9 is_stmt 0 view .LVU119
	mov	w19, w0
	.loc 1 167 3 view .LVU120
	mov	x1, 0
	mov	x0, x25
.LVL37:
	.loc 1 159 3 is_stmt 1 view .LVU121
	.loc 1 159 6 is_stmt 0 view .LVU122
	cbz	w19, .L27
	.loc 1 162 6 view .LVU123
	cmp	w19, 11
	bne	.L27
	.loc 1 166 14 discriminator 2 view .LVU124
	stp	xzr, x20, [sp, 96]
	.loc 1 167 3 is_stmt 1 discriminator 2 view .LVU125
	.loc 1 157 24 is_stmt 0 discriminator 2 view .LVU126
	add	x20, x20, x27
.LVL38:
	.loc 1 167 3 discriminator 2 view .LVU127
	bl	nanosleep
.LVL39:
	.loc 1 157 35 is_stmt 1 discriminator 2 view .LVU128
	.loc 1 157 24 discriminator 2 view .LVU129
	cmp	x20, x26
	bne	.L29
.L27:
	.loc 1 170 1 is_stmt 0 view .LVU130
	mov	w0, w19
	ldp	x19, x20, [sp, 16]
.LVL40:
	.loc 1 170 1 view .LVU131
	ldp	x21, x22, [sp, 32]
.LVL41:
	.loc 1 170 1 view .LVU132
	ldp	x23, x24, [sp, 48]
.LVL42:
	.loc 1 170 1 view .LVU133
	ldp	x25, x26, [sp, 64]
	ldr	x27, [sp, 80]
	ldp	x29, x30, [sp], 112
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 27
	.cfi_restore 25
	.cfi_restore 26
	.cfi_restore 23
	.cfi_restore 24
	.cfi_restore 21
	.cfi_restore 22
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
	.cfi_endproc
.LFE59:
	.size	_cgo_try_pthread_create, .-_cgo_try_pthread_create
	.section	.rodata.str1.8
	.align	3
.LC1:
	.string	"pthread_create failed: %s"
	.text
	.align	2
	.p2align 4,,11
	.global	x_cgo_sys_thread_create
	.type	x_cgo_sys_thread_create, %function
x_cgo_sys_thread_create:
.LVL43:
.LFB52:
	.loc 1 39 58 is_stmt 1 view -0
	.cfi_startproc
	.loc 1 40 2 view .LVU135
	.loc 1 41 2 view .LVU136
	.loc 1 43 2 view .LVU137
	.loc 1 39 58 is_stmt 0 view .LVU138
	stp	x29, x30, [sp, -128]!
	.cfi_def_cfa_offset 128
	.cfi_offset 29, -128
	.cfi_offset 30, -120
	mov	x29, sp
	stp	x19, x20, [sp, 16]
	.cfi_offset 19, -112
	.cfi_offset 20, -104
	.loc 1 43 2 view .LVU139
	add	x19, sp, 64
	.loc 1 39 58 view .LVU140
	mov	x20, x0
	.loc 1 43 2 view .LVU141
	mov	x0, x19
.LVL44:
	.loc 1 39 58 view .LVU142
	str	x21, [sp, 32]
	.cfi_offset 21, -96
	.loc 1 39 58 view .LVU143
	mov	x21, x1
	.loc 1 43 2 view .LVU144
	bl	pthread_attr_init
.LVL45:
	.loc 1 44 2 is_stmt 1 view .LVU145
	mov	w1, 1
	mov	x0, x19
	bl	pthread_attr_setdetachstate
.LVL46:
	.loc 1 45 2 view .LVU146
	.loc 1 45 12 is_stmt 0 view .LVU147
	mov	x3, x21
	mov	x2, x20
	mov	x1, x19
	add	x0, sp, 56
	bl	_cgo_try_pthread_create
.LVL47:
	.loc 1 46 2 is_stmt 1 view .LVU148
	.loc 1 46 5 is_stmt 0 view .LVU149
	cbnz	w0, .L38
	.loc 1 50 1 view .LVU150
	ldp	x19, x20, [sp, 16]
.LVL48:
	.loc 1 50 1 view .LVU151
	ldr	x21, [sp, 32]
.LVL49:
	.loc 1 50 1 view .LVU152
	ldp	x29, x30, [sp], 128
	.cfi_remember_state
	.cfi_restore 30
	.cfi_restore 29
	.cfi_restore 21
	.cfi_restore 19
	.cfi_restore 20
	.cfi_def_cfa_offset 0
	ret
.LVL50:
.L38:
	.cfi_restore_state
	.loc 1 47 3 is_stmt 1 view .LVU153
	adrp	x1, :got:stderr
	ldr	x1, [x1, #:got_lo12:stderr]
	ldr	x19, [x1]
	bl	strerror
.LVL51:
.LBB13:
.LBI13:
	.loc 2 103 1 view .LVU154
.LBB14:
	.loc 2 105 3 view .LVU155
	.loc 2 105 10 is_stmt 0 view .LVU156
	adrp	x2, .LC1
	mov	x3, x0
	add	x2, x2, :lo12:.LC1
	mov	w1, 1
	mov	x0, x19
	bl	__fprintf_chk
.LVL52:
	.loc 2 105 10 view .LVU157
.LBE14:
.LBE13:
	.loc 1 48 3 is_stmt 1 view .LVU158
	bl	abort
.LVL53:
	.cfi_endproc
.LFE52:
	.size	x_cgo_sys_thread_create, .-x_cgo_sys_thread_create
	.global	x_crosscall2_ptr
	.global	x_cgo_pthread_key_created
	.bss
	.align	3
	.set	.LANCHOR0,. + 0
	.type	cgo_context_function, %object
	.size	cgo_context_function, 8
cgo_context_function:
	.zero	8
	.type	runtime_init_done, %object
	.size	runtime_init_done, 4
runtime_init_done:
	.zero	4
	.zero	4
	.type	runtime_init_mu, %object
	.size	runtime_init_mu, 48
runtime_init_mu:
	.zero	48
	.type	runtime_init_cond, %object
	.size	runtime_init_cond, 48
runtime_init_cond:
	.zero	48
	.type	pthread_g, %object
	.size	pthread_g, 4
pthread_g:
	.zero	4
	.zero	4
	.type	x_crosscall2_ptr, %object
	.size	x_crosscall2_ptr, 8
x_crosscall2_ptr:
	.zero	8
	.type	x_cgo_pthread_key_created, %object
	.size	x_cgo_pthread_key_created, 8
x_cgo_pthread_key_created:
	.zero	8
	.text
.Letext0:
	.file 3 "/usr/include/aarch64-linux-gnu/bits/types.h"
	.file 4 "/usr/lib/gcc/aarch64-linux-gnu/11/include/stddef.h"
	.file 5 "/usr/include/aarch64-linux-gnu/bits/types/struct_timespec.h"
	.file 6 "/usr/include/aarch64-linux-gnu/bits/atomic_wide_counter.h"
	.file 7 "/usr/include/aarch64-linux-gnu/bits/thread-shared-types.h"
	.file 8 "/usr/include/aarch64-linux-gnu/bits/struct_mutex.h"
	.file 9 "/usr/include/aarch64-linux-gnu/bits/pthreadtypes.h"
	.file 10 "/usr/include/pthread.h"
	.file 11 "/usr/include/aarch64-linux-gnu/bits/types/struct_FILE.h"
	.file 12 "/usr/include/aarch64-linux-gnu/bits/types/FILE.h"
	.file 13 "/usr/include/stdint.h"
	.file 14 "libcgo.h"
	.file 15 "/usr/include/stdio.h"
	.file 16 "/usr/include/time.h"
	.file 17 "/usr/include/string.h"
	.file 18 "/usr/include/stdlib.h"
	.section	.debug_info,"",@progbits
.Ldebug_info0:
	.4byte	0xe04
	.2byte	0x5
	.byte	0x1
	.byte	0x8
	.4byte	.Ldebug_abbrev0
	.uleb128 0x23
	.4byte	.LASF141
	.byte	0x1d
	.4byte	.LASF0
	.4byte	.LASF1
	.8byte	.Ltext0
	.8byte	.Letext0-.Ltext0
	.4byte	.Ldebug_line0
	.uleb128 0x6
	.byte	0x8
	.byte	0x7
	.4byte	.LASF2
	.uleb128 0x6
	.byte	0x1
	.byte	0x8
	.4byte	.LASF3
	.uleb128 0x6
	.byte	0x2
	.byte	0x7
	.4byte	.LASF4
	.uleb128 0x6
	.byte	0x4
	.byte	0x7
	.4byte	.LASF5
	.uleb128 0x6
	.byte	0x1
	.byte	0x6
	.4byte	.LASF6
	.uleb128 0x6
	.byte	0x2
	.byte	0x5
	.4byte	.LASF7
	.uleb128 0x24
	.byte	0x4
	.byte	0x5
	.string	"int"
	.uleb128 0x6
	.byte	0x8
	.byte	0x5
	.4byte	.LASF8
	.uleb128 0x5
	.4byte	.LASF9
	.byte	0x3
	.byte	0x98
	.byte	0x19
	.4byte	0x5f
	.uleb128 0x5
	.4byte	.LASF10
	.byte	0x3
	.byte	0x99
	.byte	0x1b
	.4byte	0x5f
	.uleb128 0x5
	.4byte	.LASF11
	.byte	0x3
	.byte	0xa0
	.byte	0x1a
	.4byte	0x5f
	.uleb128 0x25
	.byte	0x8
	.uleb128 0xb
	.4byte	0x8a
	.uleb128 0x5
	.4byte	.LASF12
	.byte	0x3
	.byte	0xc5
	.byte	0x21
	.4byte	0x5f
	.uleb128 0x4
	.4byte	0xa2
	.uleb128 0x6
	.byte	0x1
	.byte	0x8
	.4byte	.LASF13
	.uleb128 0x14
	.4byte	0xa2
	.uleb128 0x5
	.4byte	.LASF14
	.byte	0x4
	.byte	0xd1
	.byte	0x17
	.4byte	0x2e
	.uleb128 0xd
	.4byte	.LASF23
	.byte	0x10
	.byte	0x5
	.byte	0xb
	.byte	0x8
	.4byte	0xe2
	.uleb128 0x1
	.4byte	.LASF15
	.byte	0x5
	.byte	0x10
	.byte	0xc
	.4byte	0x7e
	.byte	0
	.uleb128 0x1
	.4byte	.LASF16
	.byte	0x5
	.byte	0x15
	.byte	0x15
	.4byte	0x91
	.byte	0x8
	.byte	0
	.uleb128 0x14
	.4byte	0xba
	.uleb128 0x4
	.4byte	0xa9
	.uleb128 0xb
	.4byte	0xe7
	.uleb128 0x26
	.byte	0x8
	.byte	0x6
	.byte	0x1c
	.byte	0x3
	.4byte	0x115
	.uleb128 0x1
	.4byte	.LASF17
	.byte	0x6
	.byte	0x1e
	.byte	0x12
	.4byte	0x43
	.byte	0
	.uleb128 0x1
	.4byte	.LASF18
	.byte	0x6
	.byte	0x1f
	.byte	0x12
	.4byte	0x43
	.byte	0x4
	.byte	0
	.uleb128 0x15
	.byte	0x8
	.byte	0x6
	.byte	0x19
	.4byte	0x136
	.uleb128 0x7
	.4byte	.LASF19
	.byte	0x6
	.byte	0x1b
	.byte	0x28
	.4byte	0x136
	.uleb128 0x7
	.4byte	.LASF20
	.byte	0x6
	.byte	0x20
	.byte	0x5
	.4byte	0xf1
	.byte	0
	.uleb128 0x6
	.byte	0x8
	.byte	0x7
	.4byte	.LASF21
	.uleb128 0x5
	.4byte	.LASF22
	.byte	0x6
	.byte	0x21
	.byte	0x3
	.4byte	0x115
	.uleb128 0xd
	.4byte	.LASF24
	.byte	0x10
	.byte	0x7
	.byte	0x33
	.byte	0x10
	.4byte	0x171
	.uleb128 0x1
	.4byte	.LASF25
	.byte	0x7
	.byte	0x35
	.byte	0x23
	.4byte	0x171
	.byte	0
	.uleb128 0x1
	.4byte	.LASF26
	.byte	0x7
	.byte	0x36
	.byte	0x23
	.4byte	0x171
	.byte	0x8
	.byte	0
	.uleb128 0x4
	.4byte	0x149
	.uleb128 0x5
	.4byte	.LASF27
	.byte	0x7
	.byte	0x37
	.byte	0x3
	.4byte	0x149
	.uleb128 0xd
	.4byte	.LASF28
	.byte	0x28
	.byte	0x8
	.byte	0x1b
	.byte	0x8
	.4byte	0x1eb
	.uleb128 0x1
	.4byte	.LASF29
	.byte	0x8
	.byte	0x1d
	.byte	0x7
	.4byte	0x58
	.byte	0
	.uleb128 0x1
	.4byte	.LASF30
	.byte	0x8
	.byte	0x1e
	.byte	0x10
	.4byte	0x43
	.byte	0x4
	.uleb128 0x1
	.4byte	.LASF31
	.byte	0x8
	.byte	0x1f
	.byte	0x7
	.4byte	0x58
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF32
	.byte	0x8
	.byte	0x21
	.byte	0x10
	.4byte	0x43
	.byte	0xc
	.uleb128 0x1
	.4byte	.LASF33
	.byte	0x8
	.byte	0x3a
	.byte	0x7
	.4byte	0x58
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF34
	.byte	0x8
	.byte	0x3f
	.byte	0x7
	.4byte	0x58
	.byte	0x14
	.uleb128 0x1
	.4byte	.LASF35
	.byte	0x8
	.byte	0x40
	.byte	0x14
	.4byte	0x176
	.byte	0x18
	.byte	0
	.uleb128 0xd
	.4byte	.LASF36
	.byte	0x30
	.byte	0x7
	.byte	0x5e
	.byte	0x8
	.4byte	0x254
	.uleb128 0x1
	.4byte	.LASF37
	.byte	0x7
	.byte	0x60
	.byte	0x19
	.4byte	0x13d
	.byte	0
	.uleb128 0x1
	.4byte	.LASF38
	.byte	0x7
	.byte	0x61
	.byte	0x19
	.4byte	0x13d
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF39
	.byte	0x7
	.byte	0x62
	.byte	0x10
	.4byte	0x254
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF40
	.byte	0x7
	.byte	0x63
	.byte	0x10
	.4byte	0x254
	.byte	0x18
	.uleb128 0x1
	.4byte	.LASF41
	.byte	0x7
	.byte	0x64
	.byte	0x10
	.4byte	0x43
	.byte	0x20
	.uleb128 0x1
	.4byte	.LASF42
	.byte	0x7
	.byte	0x65
	.byte	0x10
	.4byte	0x43
	.byte	0x24
	.uleb128 0x1
	.4byte	.LASF43
	.byte	0x7
	.byte	0x66
	.byte	0x10
	.4byte	0x254
	.byte	0x28
	.byte	0
	.uleb128 0xe
	.4byte	0x43
	.4byte	0x264
	.uleb128 0xf
	.4byte	0x2e
	.byte	0x1
	.byte	0
	.uleb128 0x5
	.4byte	.LASF44
	.byte	0x9
	.byte	0x1b
	.byte	0x1b
	.4byte	0x2e
	.uleb128 0x5
	.4byte	.LASF45
	.byte	0x9
	.byte	0x31
	.byte	0x16
	.4byte	0x43
	.uleb128 0x27
	.4byte	.LASF48
	.byte	0x40
	.byte	0x9
	.byte	0x38
	.byte	0x7
	.4byte	0x2a2
	.uleb128 0x7
	.4byte	.LASF46
	.byte	0x9
	.byte	0x3a
	.byte	0x8
	.4byte	0x2a2
	.uleb128 0x7
	.4byte	.LASF47
	.byte	0x9
	.byte	0x3b
	.byte	0xc
	.4byte	0x5f
	.byte	0
	.uleb128 0xe
	.4byte	0xa2
	.4byte	0x2b2
	.uleb128 0xf
	.4byte	0x2e
	.byte	0x3f
	.byte	0
	.uleb128 0x5
	.4byte	.LASF48
	.byte	0x9
	.byte	0x3e
	.byte	0x1e
	.4byte	0x27c
	.uleb128 0x14
	.4byte	0x2b2
	.uleb128 0x15
	.byte	0x30
	.byte	0x9
	.byte	0x43
	.4byte	0x2f0
	.uleb128 0x7
	.4byte	.LASF49
	.byte	0x9
	.byte	0x45
	.byte	0x1c
	.4byte	0x182
	.uleb128 0x7
	.4byte	.LASF46
	.byte	0x9
	.byte	0x46
	.byte	0x8
	.4byte	0x2f0
	.uleb128 0x7
	.4byte	.LASF47
	.byte	0x9
	.byte	0x47
	.byte	0xc
	.4byte	0x5f
	.byte	0
	.uleb128 0xe
	.4byte	0xa2
	.4byte	0x300
	.uleb128 0xf
	.4byte	0x2e
	.byte	0x2f
	.byte	0
	.uleb128 0x5
	.4byte	.LASF50
	.byte	0x9
	.byte	0x48
	.byte	0x3
	.4byte	0x2c3
	.uleb128 0x15
	.byte	0x30
	.byte	0x9
	.byte	0x4b
	.4byte	0x339
	.uleb128 0x7
	.4byte	.LASF49
	.byte	0x9
	.byte	0x4d
	.byte	0x1b
	.4byte	0x1eb
	.uleb128 0x7
	.4byte	.LASF46
	.byte	0x9
	.byte	0x4e
	.byte	0x8
	.4byte	0x2f0
	.uleb128 0x7
	.4byte	.LASF47
	.byte	0x9
	.byte	0x4f
	.byte	0x1f
	.4byte	0x339
	.byte	0
	.uleb128 0x6
	.byte	0x8
	.byte	0x5
	.4byte	.LASF51
	.uleb128 0x5
	.4byte	.LASF52
	.byte	0x9
	.byte	0x50
	.byte	0x3
	.4byte	0x30c
	.uleb128 0x1c
	.4byte	0x43
	.byte	0x26
	.4byte	0x363
	.uleb128 0x8
	.4byte	.LASF53
	.byte	0
	.uleb128 0x8
	.4byte	.LASF54
	.byte	0x1
	.byte	0
	.uleb128 0x1c
	.4byte	0x43
	.byte	0x30
	.4byte	0x39e
	.uleb128 0x8
	.4byte	.LASF55
	.byte	0
	.uleb128 0x8
	.4byte	.LASF56
	.byte	0x1
	.uleb128 0x8
	.4byte	.LASF57
	.byte	0x2
	.uleb128 0x8
	.4byte	.LASF58
	.byte	0x3
	.uleb128 0x8
	.4byte	.LASF59
	.byte	0
	.uleb128 0x8
	.4byte	.LASF60
	.byte	0x1
	.uleb128 0x8
	.4byte	.LASF61
	.byte	0x2
	.uleb128 0x8
	.4byte	.LASF62
	.byte	0
	.byte	0
	.uleb128 0x16
	.4byte	0x3a9
	.uleb128 0x3
	.4byte	0x8a
	.byte	0
	.uleb128 0x4
	.4byte	0x39e
	.uleb128 0xd
	.4byte	.LASF63
	.byte	0xd8
	.byte	0xb
	.byte	0x31
	.byte	0x8
	.4byte	0x535
	.uleb128 0x1
	.4byte	.LASF64
	.byte	0xb
	.byte	0x33
	.byte	0x7
	.4byte	0x58
	.byte	0
	.uleb128 0x1
	.4byte	.LASF65
	.byte	0xb
	.byte	0x36
	.byte	0x9
	.4byte	0x9d
	.byte	0x8
	.uleb128 0x1
	.4byte	.LASF66
	.byte	0xb
	.byte	0x37
	.byte	0x9
	.4byte	0x9d
	.byte	0x10
	.uleb128 0x1
	.4byte	.LASF67
	.byte	0xb
	.byte	0x38
	.byte	0x9
	.4byte	0x9d
	.byte	0x18
	.uleb128 0x1
	.4byte	.LASF68
	.byte	0xb
	.byte	0x39
	.byte	0x9
	.4byte	0x9d
	.byte	0x20
	.uleb128 0x1
	.4byte	.LASF69
	.byte	0xb
	.byte	0x3a
	.byte	0x9
	.4byte	0x9d
	.byte	0x28
	.uleb128 0x1
	.4byte	.LASF70
	.byte	0xb
	.byte	0x3b
	.byte	0x9
	.4byte	0x9d
	.byte	0x30
	.uleb128 0x1
	.4byte	.LASF71
	.byte	0xb
	.byte	0x3c
	.byte	0x9
	.4byte	0x9d
	.byte	0x38
	.uleb128 0x1
	.4byte	.LASF72
	.byte	0xb
	.byte	0x3d
	.byte	0x9
	.4byte	0x9d
	.byte	0x40
	.uleb128 0x1
	.4byte	.LASF73
	.byte	0xb
	.byte	0x40
	.byte	0x9
	.4byte	0x9d
	.byte	0x48
	.uleb128 0x1
	.4byte	.LASF74
	.byte	0xb
	.byte	0x41
	.byte	0x9
	.4byte	0x9d
	.byte	0x50
	.uleb128 0x1
	.4byte	.LASF75
	.byte	0xb
	.byte	0x42
	.byte	0x9
	.4byte	0x9d
	.byte	0x58
	.uleb128 0x1
	.4byte	.LASF76
	.byte	0xb
	.byte	0x44
	.byte	0x16
	.4byte	0x54e
	.byte	0x60
	.uleb128 0x1
	.4byte	.LASF77
	.byte	0xb
	.byte	0x46
	.byte	0x14
	.4byte	0x553
	.byte	0x68
	.uleb128 0x1
	.4byte	.LASF78
	.byte	0xb
	.byte	0x48
	.byte	0x7
	.4byte	0x58
	.byte	0x70
	.uleb128 0x1
	.4byte	.LASF79
	.byte	0xb
	.byte	0x49
	.byte	0x7
	.4byte	0x58
	.byte	0x74
	.uleb128 0x1
	.4byte	.LASF80
	.byte	0xb
	.byte	0x4a
	.byte	0xb
	.4byte	0x66
	.byte	0x78
	.uleb128 0x1
	.4byte	.LASF81
	.byte	0xb
	.byte	0x4d
	.byte	0x12
	.4byte	0x3c
	.byte	0x80
	.uleb128 0x1
	.4byte	.LASF82
	.byte	0xb
	.byte	0x4e
	.byte	0xf
	.4byte	0x4a
	.byte	0x82
	.uleb128 0x1
	.4byte	.LASF83
	.byte	0xb
	.byte	0x4f
	.byte	0x8
	.4byte	0x558
	.byte	0x83
	.uleb128 0x1
	.4byte	.LASF84
	.byte	0xb
	.byte	0x51
	.byte	0xf
	.4byte	0x568
	.byte	0x88
	.uleb128 0x1
	.4byte	.LASF85
	.byte	0xb
	.byte	0x59
	.byte	0xd
	.4byte	0x72
	.byte	0x90
	.uleb128 0x1
	.4byte	.LASF86
	.byte	0xb
	.byte	0x5b
	.byte	0x17
	.4byte	0x572
	.byte	0x98
	.uleb128 0x1
	.4byte	.LASF87
	.byte	0xb
	.byte	0x5c
	.byte	0x19
	.4byte	0x57c
	.byte	0xa0
	.uleb128 0x1
	.4byte	.LASF88
	.byte	0xb
	.byte	0x5d
	.byte	0x14
	.4byte	0x553
	.byte	0xa8
	.uleb128 0x1
	.4byte	.LASF89
	.byte	0xb
	.byte	0x5e
	.byte	0x9
	.4byte	0x8a
	.byte	0xb0
	.uleb128 0x1
	.4byte	.LASF90
	.byte	0xb
	.byte	0x5f
	.byte	0xa
	.4byte	0xae
	.byte	0xb8
	.uleb128 0x1
	.4byte	.LASF91
	.byte	0xb
	.byte	0x60
	.byte	0x7
	.4byte	0x58
	.byte	0xc0
	.uleb128 0x1
	.4byte	.LASF92
	.byte	0xb
	.byte	0x62
	.byte	0x8
	.4byte	0x581
	.byte	0xc4
	.byte	0
	.uleb128 0x5
	.4byte	.LASF93
	.byte	0xc
	.byte	0x7
	.byte	0x19
	.4byte	0x3ae
	.uleb128 0x28
	.4byte	.LASF142
	.byte	0xb
	.byte	0x2b
	.byte	0xe
	.uleb128 0x17
	.4byte	.LASF94
	.uleb128 0x4
	.4byte	0x549
	.uleb128 0x4
	.4byte	0x3ae
	.uleb128 0xe
	.4byte	0xa2
	.4byte	0x568
	.uleb128 0xf
	.4byte	0x2e
	.byte	0
	.byte	0
	.uleb128 0x4
	.4byte	0x541
	.uleb128 0x17
	.4byte	.LASF95
	.uleb128 0x4
	.4byte	0x56d
	.uleb128 0x17
	.4byte	.LASF96
	.uleb128 0x4
	.4byte	0x577
	.uleb128 0xe
	.4byte	0xa2
	.4byte	0x591
	.uleb128 0xf
	.4byte	0x2e
	.byte	0x13
	.byte	0
	.uleb128 0x4
	.4byte	0x535
	.uleb128 0xb
	.4byte	0x591
	.uleb128 0x29
	.4byte	.LASF107
	.byte	0xf
	.byte	0x91
	.byte	0xe
	.4byte	0x591
	.uleb128 0x4
	.4byte	0x5ac
	.uleb128 0x2a
	.uleb128 0x5
	.4byte	.LASF97
	.byte	0xd
	.byte	0x5a
	.byte	0x1b
	.4byte	0x2e
	.uleb128 0x5
	.4byte	.LASF98
	.byte	0xe
	.byte	0xf
	.byte	0x13
	.4byte	0x5ad
	.uleb128 0x2b
	.string	"G"
	.byte	0xe
	.byte	0x16
	.byte	0x12
	.4byte	0x5cf
	.uleb128 0x2c
	.string	"G"
	.byte	0x10
	.byte	0xe
	.byte	0x17
	.byte	0x8
	.4byte	0x5f5
	.uleb128 0x1
	.4byte	.LASF99
	.byte	0xe
	.byte	0x19
	.byte	0xa
	.4byte	0x5b9
	.byte	0
	.uleb128 0x1
	.4byte	.LASF100
	.byte	0xe
	.byte	0x1a
	.byte	0xa
	.4byte	0x5b9
	.byte	0x8
	.byte	0
	.uleb128 0x4
	.4byte	0x5c5
	.uleb128 0x4
	.4byte	0x5b9
	.uleb128 0x4
	.4byte	0x604
	.uleb128 0x2d
	.4byte	0x8a
	.4byte	0x613
	.uleb128 0x3
	.4byte	0x8a
	.byte	0
	.uleb128 0xd
	.4byte	.LASF101
	.byte	0x8
	.byte	0xe
	.byte	0x5e
	.byte	0x8
	.4byte	0x62e
	.uleb128 0x1
	.4byte	.LASF102
	.byte	0xe
	.byte	0x5f
	.byte	0xc
	.4byte	0x5ad
	.byte	0
	.byte	0
	.uleb128 0xc
	.4byte	.LASF103
	.byte	0x17
	.byte	0x17
	.4byte	0x340
	.uleb128 0x9
	.byte	0x3
	.8byte	runtime_init_cond
	.uleb128 0xc
	.4byte	.LASF104
	.byte	0x18
	.byte	0x18
	.4byte	0x300
	.uleb128 0x9
	.byte	0x3
	.8byte	runtime_init_mu
	.uleb128 0xc
	.4byte	.LASF105
	.byte	0x19
	.byte	0xc
	.4byte	0x58
	.uleb128 0x9
	.byte	0x3
	.8byte	runtime_init_done
	.uleb128 0xc
	.4byte	.LASF106
	.byte	0x1e
	.byte	0x16
	.4byte	0x270
	.uleb128 0x9
	.byte	0x3
	.8byte	pthread_g
	.uleb128 0x1d
	.4byte	.LASF108
	.byte	0x20
	.byte	0xb
	.4byte	0x5ad
	.uleb128 0x9
	.byte	0x3
	.8byte	x_cgo_pthread_key_created
	.uleb128 0x16
	.4byte	0x6b1
	.uleb128 0x3
	.4byte	0x3a9
	.uleb128 0x3
	.4byte	0x8a
	.uleb128 0x3
	.4byte	0x58
	.uleb128 0x3
	.4byte	0xae
	.byte	0
	.uleb128 0x1d
	.4byte	.LASF109
	.byte	0x21
	.byte	0x8
	.4byte	0x6c6
	.uleb128 0x9
	.byte	0x3
	.8byte	x_crosscall2_ptr
	.uleb128 0x4
	.4byte	0x697
	.uleb128 0x16
	.4byte	0x6d6
	.uleb128 0x3
	.4byte	0x6d6
	.byte	0
	.uleb128 0x4
	.4byte	0x613
	.uleb128 0xc
	.4byte	.LASF110
	.byte	0x24
	.byte	0xf
	.4byte	0x6f0
	.uleb128 0x9
	.byte	0x3
	.8byte	cgo_context_function
	.uleb128 0x4
	.4byte	0x6cb
	.uleb128 0x9
	.4byte	.LASF111
	.byte	0x10
	.2byte	0x110
	.byte	0xc
	.4byte	0x58
	.4byte	0x711
	.uleb128 0x3
	.4byte	0x711
	.uleb128 0x3
	.4byte	0x716
	.byte	0
	.uleb128 0x4
	.4byte	0xe2
	.uleb128 0x4
	.4byte	0xba
	.uleb128 0x1e
	.4byte	.LASF112
	.byte	0xa
	.byte	0xca
	.4byte	0x58
	.4byte	0x73f
	.uleb128 0x3
	.4byte	0x744
	.uleb128 0x3
	.4byte	0x74e
	.uleb128 0x3
	.4byte	0x5ff
	.uleb128 0x3
	.4byte	0x8c
	.byte	0
	.uleb128 0x4
	.4byte	0x264
	.uleb128 0xb
	.4byte	0x73f
	.uleb128 0x4
	.4byte	0x2be
	.uleb128 0xb
	.4byte	0x749
	.uleb128 0x9
	.4byte	.LASF113
	.byte	0xa
	.2byte	0x465
	.byte	0xc
	.4byte	0x58
	.4byte	0x76a
	.uleb128 0x3
	.4byte	0x76a
	.byte	0
	.uleb128 0x4
	.4byte	0x340
	.uleb128 0xb
	.4byte	0x76a
	.uleb128 0x9
	.4byte	.LASF114
	.byte	0xa
	.2byte	0x51c
	.byte	0xc
	.4byte	0x58
	.4byte	0x790
	.uleb128 0x3
	.4byte	0x270
	.uleb128 0x3
	.4byte	0x5a7
	.byte	0
	.uleb128 0x2e
	.4byte	.LASF143
	.byte	0xe
	.byte	0x4a
	.byte	0x6
	.4byte	0x7a2
	.uleb128 0x3
	.4byte	0x5fa
	.byte	0
	.uleb128 0x9
	.4byte	.LASF115
	.byte	0xa
	.2byte	0x343
	.byte	0xc
	.4byte	0x58
	.4byte	0x7b9
	.uleb128 0x3
	.4byte	0x7b9
	.byte	0
	.uleb128 0x4
	.4byte	0x300
	.uleb128 0xb
	.4byte	0x7b9
	.uleb128 0x9
	.4byte	.LASF116
	.byte	0xa
	.2byte	0x511
	.byte	0xc
	.4byte	0x58
	.4byte	0x7df
	.uleb128 0x3
	.4byte	0x7df
	.uleb128 0x3
	.4byte	0x3a9
	.byte	0
	.uleb128 0x4
	.4byte	0x270
	.uleb128 0x9
	.4byte	.LASF117
	.byte	0xa
	.2byte	0x46d
	.byte	0xc
	.4byte	0x58
	.4byte	0x800
	.uleb128 0x3
	.4byte	0x76f
	.uleb128 0x3
	.4byte	0x7be
	.byte	0
	.uleb128 0x9
	.4byte	.LASF118
	.byte	0xa
	.2byte	0x31a
	.byte	0xc
	.4byte	0x58
	.4byte	0x817
	.uleb128 0x3
	.4byte	0x7b9
	.byte	0
	.uleb128 0x1e
	.4byte	.LASF119
	.byte	0x2
	.byte	0x5d
	.4byte	0x58
	.4byte	0x837
	.uleb128 0x3
	.4byte	0x596
	.uleb128 0x3
	.4byte	0x58
	.uleb128 0x3
	.4byte	0xec
	.uleb128 0x1f
	.byte	0
	.uleb128 0x2f
	.4byte	.LASF144
	.byte	0x12
	.2byte	0x256
	.byte	0xd
	.uleb128 0x9
	.4byte	.LASF120
	.byte	0x11
	.2byte	0x1a3
	.byte	0xe
	.4byte	0x9d
	.4byte	0x857
	.uleb128 0x3
	.4byte	0x58
	.byte	0
	.uleb128 0x9
	.4byte	.LASF121
	.byte	0xa
	.2byte	0x129
	.byte	0xc
	.4byte	0x58
	.4byte	0x873
	.uleb128 0x3
	.4byte	0x873
	.uleb128 0x3
	.4byte	0x58
	.byte	0
	.uleb128 0x4
	.4byte	0x2b2
	.uleb128 0x9
	.4byte	.LASF122
	.byte	0xa
	.2byte	0x11d
	.byte	0xc
	.4byte	0x58
	.4byte	0x88f
	.uleb128 0x3
	.4byte	0x873
	.byte	0
	.uleb128 0x30
	.4byte	.LASF145
	.byte	0x1
	.byte	0xad
	.byte	0x1
	.8byte	.LFB60
	.8byte	.LFE60-.LFB60
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x8df
	.uleb128 0x10
	.string	"g"
	.byte	0xad
	.byte	0x1e
	.4byte	0x8a
	.4byte	.LLST0
	.4byte	.LVUS0
	.uleb128 0x31
	.8byte	.LVL2
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x1
	.byte	0x30
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x1
	.byte	0x30
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x1
	.byte	0x30
	.byte	0
	.byte	0
	.uleb128 0x20
	.4byte	.LASF133
	.byte	0x98
	.4byte	0x58
	.8byte	.LFB59
	.8byte	.LFE59-.LFB59
	.uleb128 0x1
	.byte	0x9c
	.4byte	0x9c3
	.uleb128 0x11
	.4byte	.LASF123
	.byte	0x98
	.byte	0x24
	.4byte	0x73f
	.4byte	.LLST8
	.4byte	.LVUS8
	.uleb128 0x11
	.4byte	.LASF124
	.byte	0x98
	.byte	0x42
	.4byte	0x749
	.4byte	.LLST9
	.4byte	.LVUS9
	.uleb128 0x10
	.string	"pfn"
	.byte	0x98
	.byte	0x50
	.4byte	0x5ff
	.4byte	.LLST10
	.4byte	.LVUS10
	.uleb128 0x10
	.string	"arg"
	.byte	0x98
	.byte	0x63
	.4byte	0x8a
	.4byte	.LLST11
	.4byte	.LVUS11
	.uleb128 0x32
	.4byte	.LASF125
	.byte	0x1
	.byte	0x99
	.byte	0x6
	.4byte	0x58
	.4byte	.LLST12
	.4byte	.LVUS12
	.uleb128 0x18
	.string	"err"
	.byte	0x9a
	.byte	0x6
	.4byte	0x58
	.4byte	.LLST13
	.4byte	.LVUS13
	.uleb128 0x19
	.string	"ts"
	.byte	0x9b
	.byte	0x12
	.4byte	0xba
	.uleb128 0x2
	.byte	0x91
	.sleb128 -16
	.uleb128 0xa
	.8byte	.LVL36
	.4byte	0x71b
	.4byte	0x9a9
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x88
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x87
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x86
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x2
	.byte	0x85
	.sleb128 0
	.byte	0
	.uleb128 0x13
	.8byte	.LVL39
	.4byte	0x6f5
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x89
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x30
	.byte	0
	.byte	0
	.uleb128 0x33
	.4byte	.LASF146
	.byte	0x1
	.byte	0x91
	.byte	0x9
	.4byte	0x6f0
	.8byte	.LFB58
	.8byte	.LFE58-.LFB58
	.uleb128 0x1
	.byte	0x9c
	.uleb128 0x12
	.4byte	.LASF126
	.byte	0x8c
	.byte	0x6
	.8byte	.LFB57
	.8byte	.LFE57-.LFB57
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xa0d
	.uleb128 0x34
	.4byte	.LASF147
	.byte	0x1
	.byte	0x8c
	.byte	0x28
	.4byte	0x6f0
	.uleb128 0x1
	.byte	0x50
	.byte	0
	.uleb128 0x12
	.4byte	.LASF127
	.byte	0x83
	.byte	0x1
	.8byte	.LFB56
	.8byte	.LFE56-.LFB56
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xa8a
	.uleb128 0x11
	.4byte	.LASF128
	.byte	0x83
	.byte	0x26
	.4byte	0x8a
	.4byte	.LLST7
	.4byte	.LVUS7
	.uleb128 0xa
	.8byte	.LVL30
	.4byte	0x800
	.4byte	0xa55
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL31
	.4byte	0x753
	.4byte	0xa6e
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0x83
	.sleb128 64
	.byte	0
	.uleb128 0x21
	.8byte	.LVL32
	.4byte	0x7a2
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x9
	.byte	0x3
	.8byte	.LANCHOR0+16
	.byte	0
	.byte	0
	.uleb128 0x12
	.4byte	.LASF129
	.byte	0x7a
	.byte	0x6
	.8byte	.LFB55
	.8byte	.LFE55-.LFB55
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xace
	.uleb128 0x10
	.string	"g"
	.byte	0x7a
	.byte	0x18
	.4byte	0x8a
	.4byte	.LLST6
	.4byte	.LVUS6
	.uleb128 0x21
	.8byte	.LVL27
	.4byte	0x774
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x3
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0
	.byte	0
	.uleb128 0x12
	.4byte	.LASF130
	.byte	0x63
	.byte	0x6
	.8byte	.LFB54
	.8byte	.LFE54-.LFB54
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xb8f
	.uleb128 0x10
	.string	"g"
	.byte	0x63
	.byte	0x1a
	.4byte	0x5f5
	.4byte	.LLST2
	.4byte	.LVUS2
	.uleb128 0x11
	.4byte	.LASF131
	.byte	0x63
	.byte	0x26
	.4byte	0x5fa
	.4byte	.LLST3
	.4byte	.LVUS3
	.uleb128 0xc
	.4byte	.LASF132
	.byte	0x65
	.byte	0xa
	.4byte	0xb8f
	.uleb128 0x2
	.byte	0x91
	.sleb128 -16
	.uleb128 0x35
	.4byte	0xde2
	.8byte	.LBI7
	.byte	.LVU75
	.4byte	.LLRL4
	.byte	0x1
	.byte	0x73
	.byte	0x3
	.4byte	0xb69
	.uleb128 0x1a
	.4byte	0xdfa
	.4byte	.LLST5
	.4byte	.LVUS5
	.uleb128 0x36
	.4byte	0xdef
	.uleb128 0x13
	.8byte	.LVL23
	.4byte	0x817
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC0
	.byte	0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL19
	.4byte	0x790
	.4byte	0xb81
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0x1b
	.8byte	.LVL24
	.4byte	0x837
	.byte	0
	.uleb128 0xe
	.4byte	0x5b9
	.4byte	0xb9f
	.uleb128 0xf
	.4byte	0x2e
	.byte	0x1
	.byte	0
	.uleb128 0x20
	.4byte	.LASF134
	.byte	0x35
	.4byte	0x5ad
	.8byte	.LFB53
	.8byte	.LFE53-.LFB53
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xc90
	.uleb128 0x18
	.string	"pfn"
	.byte	0x36
	.byte	0x9
	.4byte	0x6f0
	.4byte	.LLST1
	.4byte	.LVUS1
	.uleb128 0x37
	.4byte	.LASF135
	.byte	0x1
	.byte	0x39
	.byte	0x6
	.4byte	0x58
	.byte	0x2
	.uleb128 0x38
	.8byte	.LBB6
	.8byte	.LBE6-.LBB6
	.4byte	0xc16
	.uleb128 0x19
	.string	"arg"
	.byte	0x57
	.byte	0x16
	.4byte	0x613
	.uleb128 0x2
	.byte	0x91
	.sleb128 -8
	.uleb128 0x39
	.8byte	.LVL5
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x91
	.sleb128 -8
	.byte	0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL9
	.4byte	0x800
	.4byte	0xc2e
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL10
	.4byte	0x7e4
	.4byte	0xc4c
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x86
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL12
	.4byte	0x7a2
	.4byte	0xc6d
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0xb
	.byte	0x3
	.8byte	.LANCHOR0
	.byte	0x23
	.uleb128 0x10
	.byte	0
	.uleb128 0x13
	.8byte	.LVL14
	.4byte	0x7c3
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0x87
	.sleb128 112
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x9
	.byte	0x3
	.8byte	pthread_key_destructor
	.byte	0
	.byte	0
	.uleb128 0x12
	.4byte	.LASF136
	.byte	0x27
	.byte	0x1
	.8byte	.LFB52
	.8byte	.LFE52-.LFB52
	.uleb128 0x1
	.byte	0x9c
	.4byte	0xde2
	.uleb128 0x11
	.4byte	.LASF137
	.byte	0x27
	.byte	0x21
	.4byte	0x5ff
	.4byte	.LLST14
	.4byte	.LVUS14
	.uleb128 0x10
	.string	"arg"
	.byte	0x27
	.byte	0x35
	.4byte	0x8a
	.4byte	.LLST15
	.4byte	.LVUS15
	.uleb128 0xc
	.4byte	.LASF124
	.byte	0x28
	.byte	0x11
	.4byte	0x2b2
	.uleb128 0x2
	.byte	0x91
	.sleb128 -64
	.uleb128 0x19
	.string	"p"
	.byte	0x29
	.byte	0xc
	.4byte	0x264
	.uleb128 0x3
	.byte	0x91
	.sleb128 -72
	.uleb128 0x18
	.string	"err"
	.byte	0x2d
	.byte	0x6
	.4byte	0x58
	.4byte	.LLST16
	.4byte	.LVUS16
	.uleb128 0x3a
	.4byte	0xde2
	.8byte	.LBI13
	.byte	.LVU154
	.8byte	.LBB13
	.8byte	.LBE13-.LBB13
	.byte	0x1
	.byte	0x2f
	.byte	0x3
	.4byte	0xd67
	.uleb128 0x1a
	.4byte	0xdfa
	.4byte	.LLST17
	.4byte	.LVUS17
	.uleb128 0x1a
	.4byte	0xdef
	.4byte	.LLST18
	.4byte	.LVUS18
	.uleb128 0x13
	.8byte	.LVL52
	.4byte	0x817
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x9
	.byte	0x3
	.8byte	.LC1
	.byte	0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL45
	.4byte	0x878
	.4byte	0xd7f
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.byte	0
	.uleb128 0xa
	.8byte	.LVL46
	.4byte	0x857
	.4byte	0xd9c
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x1
	.byte	0x31
	.byte	0
	.uleb128 0xa
	.8byte	.LVL47
	.4byte	0x8df
	.4byte	0xdc7
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x50
	.uleb128 0x3
	.byte	0x91
	.sleb128 -72
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x51
	.uleb128 0x2
	.byte	0x83
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x52
	.uleb128 0x2
	.byte	0x84
	.sleb128 0
	.uleb128 0x2
	.uleb128 0x1
	.byte	0x53
	.uleb128 0x2
	.byte	0x85
	.sleb128 0
	.byte	0
	.uleb128 0x1b
	.8byte	.LVL51
	.4byte	0x840
	.uleb128 0x1b
	.8byte	.LVL53
	.4byte	0x837
	.byte	0
	.uleb128 0x3b
	.4byte	.LASF138
	.byte	0x2
	.byte	0x67
	.byte	0x1
	.4byte	0x58
	.byte	0x3
	.uleb128 0x22
	.4byte	.LASF139
	.byte	0x67
	.byte	0x1b
	.4byte	0x596
	.uleb128 0x22
	.4byte	.LASF140
	.byte	0x67
	.byte	0x3c
	.4byte	0xec
	.uleb128 0x1f
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
	.uleb128 0x49
	.byte	0
	.uleb128 0x2
	.uleb128 0x18
	.uleb128 0x7e
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x3
	.uleb128 0x5
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x4
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 8
	.uleb128 0x49
	.uleb128 0x13
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
	.byte	0
	.byte	0
	.uleb128 0x8
	.uleb128 0x28
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x1c
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
	.uleb128 0x37
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xc
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
	.uleb128 0x1
	.byte	0x1
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0xf
	.uleb128 0x21
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x2f
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x10
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
	.uleb128 0x11
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
	.uleb128 0x13
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x14
	.uleb128 0x26
	.byte	0
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x15
	.uleb128 0x17
	.byte	0x1
	.uleb128 0xb
	.uleb128 0xb
	.uleb128 0x3a
	.uleb128 0xb
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 9
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x16
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x17
	.uleb128 0x13
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x18
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
	.uleb128 0x19
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
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x1a
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
	.uleb128 0x1b
	.uleb128 0x48
	.byte	0
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x7f
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1c
	.uleb128 0x4
	.byte	0x1
	.uleb128 0x3e
	.uleb128 0x21
	.sleb128 7
	.uleb128 0xb
	.uleb128 0x21
	.sleb128 4
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 10
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0x21
	.sleb128 1
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x1d
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
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x2
	.uleb128 0x18
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
	.uleb128 0x1f
	.uleb128 0x18
	.byte	0
	.byte	0
	.byte	0
	.uleb128 0x20
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
	.uleb128 0x21
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
	.uleb128 0x22
	.uleb128 0x5
	.byte	0
	.uleb128 0x3
	.uleb128 0xe
	.uleb128 0x3a
	.uleb128 0x21
	.sleb128 2
	.uleb128 0x3b
	.uleb128 0xb
	.uleb128 0x39
	.uleb128 0xb
	.uleb128 0x49
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x23
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
	.uleb128 0x24
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
	.uleb128 0x25
	.uleb128 0xf
	.byte	0
	.uleb128 0xb
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x26
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
	.uleb128 0x27
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
	.uleb128 0x28
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
	.byte	0
	.byte	0
	.uleb128 0x29
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
	.uleb128 0x3f
	.uleb128 0x19
	.uleb128 0x3c
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x2a
	.uleb128 0x26
	.byte	0
	.byte	0
	.byte	0
	.uleb128 0x2b
	.uleb128 0x16
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
	.uleb128 0x2c
	.uleb128 0x13
	.byte	0x1
	.uleb128 0x3
	.uleb128 0x8
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
	.uleb128 0x2d
	.uleb128 0x15
	.byte	0x1
	.uleb128 0x27
	.uleb128 0x19
	.uleb128 0x49
	.uleb128 0x13
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2e
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
	.uleb128 0x3c
	.uleb128 0x19
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x2f
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
	.uleb128 0x30
	.uleb128 0x2e
	.byte	0x1
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
	.uleb128 0x31
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x82
	.uleb128 0x19
	.byte	0
	.byte	0
	.uleb128 0x32
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
	.uleb128 0x17
	.uleb128 0x2137
	.uleb128 0x17
	.byte	0
	.byte	0
	.uleb128 0x33
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
	.uleb128 0x34
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
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x35
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
	.uleb128 0xb
	.uleb128 0x59
	.uleb128 0xb
	.uleb128 0x57
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x36
	.uleb128 0x5
	.byte	0
	.uleb128 0x31
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x37
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
	.uleb128 0x1c
	.uleb128 0xb
	.byte	0
	.byte	0
	.uleb128 0x38
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
	.uleb128 0x39
	.uleb128 0x48
	.byte	0x1
	.uleb128 0x7d
	.uleb128 0x1
	.uleb128 0x83
	.uleb128 0x18
	.byte	0
	.byte	0
	.uleb128 0x3a
	.uleb128 0x1d
	.byte	0x1
	.uleb128 0x31
	.uleb128 0x13
	.uleb128 0x52
	.uleb128 0x1
	.uleb128 0x2138
	.uleb128 0xb
	.uleb128 0x11
	.uleb128 0x1
	.uleb128 0x12
	.uleb128 0x7
	.uleb128 0x58
	.uleb128 0xb
	.uleb128 0x59
	.uleb128 0xb
	.uleb128 0x57
	.uleb128 0xb
	.uleb128 0x1
	.uleb128 0x13
	.byte	0
	.byte	0
	.uleb128 0x3b
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
	.uleb128 .LVU7
	.uleb128 .LVU7
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 .LVU8
	.uleb128 0
.LLST0:
	.byte	0x4
	.uleb128 .LVL0-.Ltext0
	.uleb128 .LVL1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL1-.Ltext0
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL2-1-.Ltext0
	.uleb128 .LVL2-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL2-.Ltext0
	.uleb128 .LFE60-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS8:
	.uleb128 0
	.uleb128 .LVU115
	.uleb128 .LVU115
	.uleb128 .LVU133
	.uleb128 .LVU133
	.uleb128 0
.LLST8:
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LVL42-.Ltext0
	.uleb128 0x1
	.byte	0x68
	.byte	0x4
	.uleb128 .LVL42-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS9:
	.uleb128 0
	.uleb128 .LVU115
	.uleb128 .LVU115
	.uleb128 .LVU133
	.uleb128 .LVU133
	.uleb128 0
.LLST9:
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LVL42-.Ltext0
	.uleb128 0x1
	.byte	0x67
	.byte	0x4
	.uleb128 .LVL42-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0
.LVUS10:
	.uleb128 0
	.uleb128 .LVU115
	.uleb128 .LVU115
	.uleb128 .LVU132
	.uleb128 .LVU132
	.uleb128 0
.LLST10:
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x1
	.byte	0x52
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LVL41-.Ltext0
	.uleb128 0x1
	.byte	0x66
	.byte	0x4
	.uleb128 .LVL41-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x52
	.byte	0x9f
	.byte	0
.LVUS11:
	.uleb128 0
	.uleb128 .LVU115
	.uleb128 .LVU115
	.uleb128 .LVU132
	.uleb128 .LVU132
	.uleb128 0
.LLST11:
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x1
	.byte	0x53
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LVL41-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL41-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x53
	.byte	0x9f
	.byte	0
.LVUS12:
	.uleb128 .LVU109
	.uleb128 .LVU115
	.uleb128 .LVU115
	.uleb128 .LVU127
	.uleb128 .LVU127
	.uleb128 .LVU128
.LLST12:
	.byte	0x4
	.uleb128 .LVL34-.Ltext0
	.uleb128 .LVL35-.Ltext0
	.uleb128 0x2
	.byte	0x30
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL35-.Ltext0
	.uleb128 .LVL38-.Ltext0
	.uleb128 0x11
	.byte	0x84
	.sleb128 -1000000
	.byte	0xa8
	.uleb128 0x2e
	.byte	0xc
	.4byte	0xf4240
	.byte	0xa8
	.uleb128 0x2e
	.byte	0x1b
	.byte	0xa8
	.uleb128 0
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL38-.Ltext0
	.uleb128 .LVL39-1-.Ltext0
	.uleb128 0x16
	.byte	0x91
	.sleb128 -8
	.byte	0x6
	.byte	0xc
	.4byte	0xf4240
	.byte	0x1c
	.byte	0xa8
	.uleb128 0x2e
	.byte	0xc
	.4byte	0xf4240
	.byte	0xa8
	.uleb128 0x2e
	.byte	0x1b
	.byte	0xa8
	.uleb128 0
	.byte	0x9f
	.byte	0
.LVUS13:
	.uleb128 .LVU121
	.uleb128 .LVU131
	.uleb128 .LVU131
	.uleb128 0
.LLST13:
	.byte	0x4
	.uleb128 .LVL37-.Ltext0
	.uleb128 .LVL40-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL40-.Ltext0
	.uleb128 .LFE59-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS7:
	.uleb128 0
	.uleb128 .LVU91
	.uleb128 .LVU91
	.uleb128 0
.LLST7:
	.byte	0x4
	.uleb128 .LVL28-.Ltext0
	.uleb128 .LVL29-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL29-.Ltext0
	.uleb128 .LFE56-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS6:
	.uleb128 0
	.uleb128 .LVU85
	.uleb128 .LVU85
	.uleb128 .LVU86
	.uleb128 .LVU86
	.uleb128 0
.LLST6:
	.byte	0x4
	.uleb128 .LVL25-.Ltext0
	.uleb128 .LVL26-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL26-.Ltext0
	.uleb128 .LVL27-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL27-1-.Ltext0
	.uleb128 .LFE55-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0
.LVUS2:
	.uleb128 0
	.uleb128 .LVU64
	.uleb128 .LVU64
	.uleb128 .LVU72
	.uleb128 .LVU72
	.uleb128 .LVU74
	.uleb128 .LVU74
	.uleb128 0
.LLST2:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL18-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL18-.Ltext0
	.uleb128 .LVL20-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL20-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0
.LVUS3:
	.uleb128 0
	.uleb128 .LVU61
	.uleb128 .LVU61
	.uleb128 .LVU72
	.uleb128 .LVU72
	.uleb128 .LVU73
	.uleb128 .LVU73
	.uleb128 .LVU74
	.uleb128 .LVU74
	.uleb128 0
.LLST3:
	.byte	0x4
	.uleb128 .LVL15-.Ltext0
	.uleb128 .LVL16-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL16-.Ltext0
	.uleb128 .LVL20-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL20-.Ltext0
	.uleb128 .LVL21-.Ltext0
	.uleb128 0x10
	.byte	0x91
	.sleb128 -16
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x30
	.byte	0x29
	.byte	0x28
	.2byte	0x1
	.byte	0x16
	.byte	0x13
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL21-.Ltext0
	.uleb128 .LVL22-.Ltext0
	.uleb128 0x10
	.byte	0x8f
	.sleb128 -16
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x30
	.byte	0x29
	.byte	0x28
	.2byte	0x1
	.byte	0x16
	.byte	0x13
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LFE54-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS5:
	.uleb128 .LVU75
	.uleb128 .LVU81
.LLST5:
	.byte	0x4
	.uleb128 .LVL22-.Ltext0
	.uleb128 .LVL23-.Ltext0
	.uleb128 0xa
	.byte	0x3
	.8byte	.LC0
	.byte	0x9f
	.byte	0
.LVUS1:
	.uleb128 .LVU16
	.uleb128 .LVU21
	.uleb128 .LVU21
	.uleb128 .LVU32
	.uleb128 .LVU33
	.uleb128 .LVU36
	.uleb128 .LVU49
	.uleb128 .LVU51
.LLST1:
	.byte	0x4
	.uleb128 .LVL3-.Ltext0
	.uleb128 .LVL4-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL4-.Ltext0
	.uleb128 .LVL6-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0x4
	.uleb128 .LVL7-.Ltext0
	.uleb128 .LVL8-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL11-.Ltext0
	.uleb128 .LVL13-.Ltext0
	.uleb128 0x1
	.byte	0x63
	.byte	0
.LVUS14:
	.uleb128 0
	.uleb128 .LVU142
	.uleb128 .LVU142
	.uleb128 .LVU151
	.uleb128 .LVU151
	.uleb128 .LVU153
	.uleb128 .LVU153
	.uleb128 0
.LLST14:
	.byte	0x4
	.uleb128 .LVL43-.Ltext0
	.uleb128 .LVL44-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0x4
	.uleb128 .LVL44-.Ltext0
	.uleb128 .LVL48-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0x4
	.uleb128 .LVL48-.Ltext0
	.uleb128 .LVL50-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x50
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL50-.Ltext0
	.uleb128 .LFE52-.Ltext0
	.uleb128 0x1
	.byte	0x64
	.byte	0
.LVUS15:
	.uleb128 0
	.uleb128 .LVU145
	.uleb128 .LVU145
	.uleb128 .LVU152
	.uleb128 .LVU152
	.uleb128 .LVU153
	.uleb128 .LVU153
	.uleb128 0
.LLST15:
	.byte	0x4
	.uleb128 .LVL43-.Ltext0
	.uleb128 .LVL45-1-.Ltext0
	.uleb128 0x1
	.byte	0x51
	.byte	0x4
	.uleb128 .LVL45-1-.Ltext0
	.uleb128 .LVL49-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0x4
	.uleb128 .LVL49-.Ltext0
	.uleb128 .LVL50-.Ltext0
	.uleb128 0x4
	.byte	0xa3
	.uleb128 0x1
	.byte	0x51
	.byte	0x9f
	.byte	0x4
	.uleb128 .LVL50-.Ltext0
	.uleb128 .LFE52-.Ltext0
	.uleb128 0x1
	.byte	0x65
	.byte	0
.LVUS16:
	.uleb128 .LVU148
	.uleb128 .LVU154
.LLST16:
	.byte	0x4
	.uleb128 .LVL47-.Ltext0
	.uleb128 .LVL51-1-.Ltext0
	.uleb128 0x1
	.byte	0x50
	.byte	0
.LVUS17:
	.uleb128 .LVU154
	.uleb128 .LVU157
.LLST17:
	.byte	0x4
	.uleb128 .LVL51-.Ltext0
	.uleb128 .LVL52-.Ltext0
	.uleb128 0xa
	.byte	0x3
	.8byte	.LC1
	.byte	0x9f
	.byte	0
.LVUS18:
	.uleb128 .LVU154
	.uleb128 .LVU157
.LLST18:
	.byte	0x4
	.uleb128 .LVL51-.Ltext0
	.uleb128 .LVL52-.Ltext0
	.uleb128 0x1
	.byte	0x63
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
.LLRL4:
	.byte	0x4
	.uleb128 .LBB7-.Ltext0
	.uleb128 .LBE7-.Ltext0
	.byte	0x4
	.uleb128 .LBB11-.Ltext0
	.uleb128 .LBE11-.Ltext0
	.byte	0x4
	.uleb128 .LBB12-.Ltext0
	.uleb128 .LBE12-.Ltext0
	.byte	0
.Ldebug_ranges3:
	.section	.debug_line,"",@progbits
.Ldebug_line0:
	.section	.debug_str,"MS",@progbits,1
.LASF106:
	.string	"pthread_g"
.LASF44:
	.string	"pthread_t"
.LASF83:
	.string	"_shortbuf"
.LASF142:
	.string	"_IO_lock_t"
.LASF130:
	.string	"_cgo_set_stacklo"
.LASF107:
	.string	"stderr"
.LASF72:
	.string	"_IO_buf_end"
.LASF57:
	.string	"PTHREAD_MUTEX_ERRORCHECK_NP"
.LASF134:
	.string	"_cgo_wait_runtime_init_done"
.LASF70:
	.string	"_IO_write_end"
.LASF5:
	.string	"unsigned int"
.LASF101:
	.string	"context_arg"
.LASF88:
	.string	"_freeres_list"
.LASF98:
	.string	"uintptr"
.LASF64:
	.string	"_flags"
.LASF42:
	.string	"__wrefs"
.LASF128:
	.string	"dummy"
.LASF120:
	.string	"strerror"
.LASF76:
	.string	"_markers"
.LASF112:
	.string	"pthread_create"
.LASF117:
	.string	"pthread_cond_wait"
.LASF113:
	.string	"pthread_cond_broadcast"
.LASF137:
	.string	"func"
.LASF62:
	.string	"PTHREAD_MUTEX_DEFAULT"
.LASF115:
	.string	"pthread_mutex_unlock"
.LASF118:
	.string	"pthread_mutex_lock"
.LASF146:
	.string	"_cgo_get_context_function"
.LASF52:
	.string	"pthread_cond_t"
.LASF55:
	.string	"PTHREAD_MUTEX_TIMED_NP"
.LASF24:
	.string	"__pthread_internal_list"
.LASF122:
	.string	"pthread_attr_init"
.LASF38:
	.string	"__g1_start"
.LASF75:
	.string	"_IO_save_end"
.LASF30:
	.string	"__count"
.LASF45:
	.string	"pthread_key_t"
.LASF95:
	.string	"_IO_codecvt"
.LASF108:
	.string	"x_cgo_pthread_key_created"
.LASF53:
	.string	"PTHREAD_CREATE_JOINABLE"
.LASF21:
	.string	"long long unsigned int"
.LASF133:
	.string	"_cgo_try_pthread_create"
.LASF60:
	.string	"PTHREAD_MUTEX_RECURSIVE"
.LASF31:
	.string	"__owner"
.LASF85:
	.string	"_offset"
.LASF40:
	.string	"__g_size"
.LASF136:
	.string	"x_cgo_sys_thread_create"
.LASF138:
	.string	"fprintf"
.LASF66:
	.string	"_IO_read_end"
.LASF144:
	.string	"abort"
.LASF16:
	.string	"tv_nsec"
.LASF14:
	.string	"size_t"
.LASF103:
	.string	"runtime_init_cond"
.LASF67:
	.string	"_IO_read_base"
.LASF131:
	.string	"pbounds"
.LASF111:
	.string	"nanosleep"
.LASF140:
	.string	"__fmt"
.LASF18:
	.string	"__high"
.LASF26:
	.string	"__next"
.LASF139:
	.string	"__stream"
.LASF23:
	.string	"timespec"
.LASF13:
	.string	"char"
.LASF119:
	.string	"__fprintf_chk"
.LASF91:
	.string	"_mode"
.LASF94:
	.string	"_IO_marker"
.LASF65:
	.string	"_IO_read_ptr"
.LASF61:
	.string	"PTHREAD_MUTEX_ERRORCHECK"
.LASF34:
	.string	"__spins"
.LASF25:
	.string	"__prev"
.LASF48:
	.string	"pthread_attr_t"
.LASF68:
	.string	"_IO_write_base"
.LASF35:
	.string	"__list"
.LASF51:
	.string	"long long int"
.LASF73:
	.string	"_IO_save_base"
.LASF147:
	.string	"context"
.LASF129:
	.string	"x_cgo_bindm"
.LASF12:
	.string	"__syscall_slong_t"
.LASF110:
	.string	"cgo_context_function"
.LASF89:
	.string	"_freeres_buf"
.LASF74:
	.string	"_IO_backup_base"
.LASF100:
	.string	"stackhi"
.LASF33:
	.string	"__kind"
.LASF90:
	.string	"__pad5"
.LASF20:
	.string	"__value32"
.LASF141:
	.string	"GNU C17 11.4.0"
.LASF82:
	.string	"_vtable_offset"
.LASF135:
	.string	"done"
.LASF27:
	.string	"__pthread_list_t"
.LASF58:
	.string	"PTHREAD_MUTEX_ADAPTIVE_NP"
.LASF105:
	.string	"runtime_init_done"
.LASF59:
	.string	"PTHREAD_MUTEX_NORMAL"
.LASF7:
	.string	"short int"
.LASF102:
	.string	"Context"
.LASF8:
	.string	"long int"
.LASF124:
	.string	"attr"
.LASF109:
	.string	"x_crosscall2_ptr"
.LASF96:
	.string	"_IO_wide_data"
.LASF116:
	.string	"pthread_key_create"
.LASF126:
	.string	"x_cgo_set_context_function"
.LASF49:
	.string	"__data"
.LASF22:
	.string	"__atomic_wide_counter"
.LASF54:
	.string	"PTHREAD_CREATE_DETACHED"
.LASF32:
	.string	"__nusers"
.LASF87:
	.string	"_wide_data"
.LASF84:
	.string	"_lock"
.LASF15:
	.string	"tv_sec"
.LASF19:
	.string	"__value64"
.LASF2:
	.string	"long unsigned int"
.LASF86:
	.string	"_codecvt"
.LASF80:
	.string	"_old_offset"
.LASF63:
	.string	"_IO_FILE"
.LASF104:
	.string	"runtime_init_mu"
.LASF114:
	.string	"pthread_setspecific"
.LASF123:
	.string	"thread"
.LASF50:
	.string	"pthread_mutex_t"
.LASF29:
	.string	"__lock"
.LASF39:
	.string	"__g_refs"
.LASF3:
	.string	"unsigned char"
.LASF99:
	.string	"stacklo"
.LASF145:
	.string	"pthread_key_destructor"
.LASF69:
	.string	"_IO_write_ptr"
.LASF56:
	.string	"PTHREAD_MUTEX_RECURSIVE_NP"
.LASF125:
	.string	"tries"
.LASF36:
	.string	"__pthread_cond_s"
.LASF11:
	.string	"__time_t"
.LASF127:
	.string	"x_cgo_notify_runtime_init_done"
.LASF43:
	.string	"__g_signals"
.LASF37:
	.string	"__wseq"
.LASF17:
	.string	"__low"
.LASF78:
	.string	"_fileno"
.LASF9:
	.string	"__off_t"
.LASF6:
	.string	"signed char"
.LASF132:
	.string	"bounds"
.LASF4:
	.string	"short unsigned int"
.LASF97:
	.string	"uintptr_t"
.LASF41:
	.string	"__g1_orig_size"
.LASF143:
	.string	"x_cgo_getstackbound"
.LASF47:
	.string	"__align"
.LASF77:
	.string	"_chain"
.LASF93:
	.string	"FILE"
.LASF79:
	.string	"_flags2"
.LASF46:
	.string	"__size"
.LASF81:
	.string	"_cur_column"
.LASF10:
	.string	"__off64_t"
.LASF121:
	.string	"pthread_attr_setdetachstate"
.LASF92:
	.string	"_unused2"
.LASF71:
	.string	"_IO_buf_base"
.LASF28:
	.string	"__pthread_mutex_s"
	.section	.debug_line_str,"MS",@progbits,1
.LASF1:
	.string	"/_/GOROOT/src/runtime/cgo"
.LASF0:
	.string	"gcc_libinit.c"
	.ident	"GCC: (Ubuntu 11.4.0-1ubuntu1~22.04.3) 11.4.0"
	.section	.note.GNU-stack,"",@progbits
