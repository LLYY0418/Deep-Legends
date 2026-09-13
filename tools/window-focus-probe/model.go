package main

import "fmt"

const version = "1.0.1"

// The protocol travels only over anonymous pipes to our own child executable.
// No arbitrary process, window, LCU endpoint or installation path is accepted.
type command struct {
	ID       int    `json:"id"`
	Op       string `json:"op"`
	OwnerPID uint32 `json:"owner_pid,omitempty"`
	Anchor   uint64 `json:"anchor,omitempty"`
}

type snapshot struct {
	Valid            bool   `json:"target_valid"`
	Visible          bool   `json:"ws_visible"`
	Minimized        bool   `json:"minimized"`
	Topmost          bool   `json:"topmost"`
	DWMOK            bool   `json:"dwm_read_ok"`
	DWMResult        string `json:"dwm_hresult"`
	Cloaked          uint32 `json:"cloaked_bits"`
	Foreground       string `json:"foreground_category"`
	ForegroundAnchor bool   `json:"foreground_is_exact_anchor"`
	RelativeZ        string `json:"relative_to_anchor"`
	InputOK          bool   `json:"last_input_available"`
	InputTick        uint32 `json:"last_input_tick"`
}

func (s snapshot) presentable() bool {
	return s.Valid && s.Visible && !s.Minimized && s.DWMOK && s.Cloaked == 0
}

type apiEvent struct {
	At       string   `json:"at_utc"`
	API      string   `json:"api"`
	Result   string   `json:"result"`
	Meaning  string   `json:"result_meaning"`
	Snapshot snapshot `json:"after_call"`
}

type response struct {
	ID     int        `json:"id"`
	PID    uint32     `json:"pid,omitempty"`
	HWND   uint64     `json:"hwnd,omitempty"`
	OK     bool       `json:"ok"`
	Error  string     `json:"error,omitempty"`
	HR     string     `json:"hresult,omitempty"`
	Events []apiEvent `json:"events,omitempty"`
}

type trial struct {
	Name           string     `json:"name"`
	Complete       bool       `json:"complete"`
	CloakAttempt   string     `json:"cloak_hresult,omitempty"`
	CloakApplied   bool       `json:"cloak_readback_applied"`
	AnchorPrepared bool       `json:"anchor_foreground_before_burst"`
	Before         snapshot   `json:"before_burst"`
	Samples        []snapshot `json:"samples"`
	Bursts         int        `json:"completed_bursts"`
	Error          string     `json:"error,omitempty"`
	SkippedReason  string     `json:"burst_skipped_reason,omitempty"`
}

func (t trial) inputChanged() bool {
	for _, s := range t.Samples {
		if !t.Before.InputOK || !s.InputOK || s.InputTick != t.Before.InputTick {
			return true
		}
	}
	return false
}

func (t trial) observations() (shown, focus, zChanged, unknown bool) {
	if len(t.Samples) == 0 {
		unknown = true
	}
	for _, s := range t.Samples {
		shown = shown || s.presentable()
		focus = focus || s.Foreground == "probe"
		zChanged = zChanged || s.Topmost != t.Before.Topmost ||
			(s.RelativeZ != "unknown" && t.Before.RelativeZ != "unknown" && s.RelativeZ != t.Before.RelativeZ)
		unknown = unknown || !s.Valid || !s.DWMOK || s.Foreground == "none" || s.Foreground == "other" ||
			(s.Foreground == "controller" && !s.ForegroundAnchor) ||
			s.RelativeZ == "unknown" || t.Before.RelativeZ == "unknown"
	}
	return
}

type finding struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

func assess(trials []trial, cleanupOK bool) finding {
	byName := make(map[string]trial)
	for _, t := range trials {
		byName[t.Name] = t
	}
	b, bok := byName["baseline"]
	e, eok := byName["external_cloak"]
	s, sok := byName["self_cloak"]
	r, rok := byName["release_show"]
	if !cleanupOK {
		return finding{"cleanup_incomplete", "测试未正常收尾；请查看错误记录。未得出可用方案。"}
	}
	if !bok || !eok || !sok || !rok || !b.Complete || !e.Complete || !s.Complete || !r.Complete {
		return finding{"incomplete", "测试取消或未完成；不能判定抑制成功。"}
	}
	if e.CloakAttempt != "0x00000000" {
		return finding{"external_cloak_rejected", "本机对模拟窗口的外部遮蔽请求被拒绝/接口不可用；这个候选方案未成立，不能据此接入客户端。"}
	}
	if !e.CloakApplied {
		return finding{"external_cloak_not_applied", "接口返回后未读回 APP 遮蔽状态；不能视为抑制生效。"}
	}
	bs, bf, _, bu := b.observations()
	rs, _, _, ru := r.observations()
	ss, _, _, su := s.observations()
	if !b.AnchorPrepared || !e.AnchorPrepared || !bs || !bf || bu || !rs || ru ||
		!r.AnchorPrepared || !s.AnchorPrepared || s.CloakAttempt != "0x00000000" || !s.CloakApplied || ss || su ||
		b.inputChanged() || e.inputChanged() || r.inputChanged() || s.inputChanged() || e.Bursts != 3 || s.Bursts != 1 ||
		b.SkippedReason != "" || e.SkippedReason != "" || s.SkippedReason != "" || r.SkippedReason != "" {
		return finding{"inconclusive", "对照、恢复、焦点或输入条件不足；不能可靠判定。请结合完整报告。"}
	}
	es, ef, ez, eu := e.observations()
	if es || ef || ez {
		return finding{"requirement_not_met", fmt.Sprintf("未满足要求：可显示=%t，获得焦点=%t，层级变化=%t。隐藏画面不等于阻止激活/置顶。", es, ef, ez)}
	}
	if eu {
		return finding{"inconclusive", "部分状态读取失败；缺失数据不等于窗口被成功抑制。"}
	}
	return finding{"candidate_only", "本次模拟未观察到显示、抢焦点或层级变化；仅是候选结果，不证明零闪烁，也不代表真实 LOL 已修复。"}
}
