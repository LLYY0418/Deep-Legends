package main

// R130 P1-6：收藏页卡片图的看门狗超时诊断。
//
// 收藏页第一层的图片名额曾经泄漏过：名额被占满后再也没有释放，整屏卡片永久
// 停在「加载中」，而后端日志里一条痕迹都没有——card_image_stalled 当时不在
// clientDiagnosticEvents 白名单里，前端就算上报也会被 400 拒收，日志里只剩一条
// client_diagnostic_rejected，「到底卡没卡住」这个问题永远查不出来。
//
// 现在前端在卡片图迟迟不出图时会补一条 watchdog 上报，下一份诊断日志就能直接
// 看出名额有没有再次泄漏（active_card_images 顶在上限、queued 一直不降）。
// 口径与 R127 的图片计时字段完全一致：只记队列计数、候选序号与来源类别枚举，
// 不记任何资源路径。
//
// 对抗变异：把 "card_image_stalled" 从 clientDiagnosticEvents 里删掉，
// TestR130CardImageStalledReachesLog 与 TestR130CardImageStalledIsWhitelisted
// 必须 FAIL（状态码变 400，日志里只剩 client_diagnostic_rejected）；把
// imageSource 的枚举 switch 去掉，TestR130CardImageStalledNeverLeaksResourcePaths
// 必须 FAIL（整条资源路径原样落盘）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestR130CardImageStalledReachesLog(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":8,"queued":6,` +
		`"sourceIndex":2,"imageSource":"gtimg"}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204：卡片图看门狗仍被白名单拒收", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events[0]
	if event["event"] != "card_image_stalled" || event["reason"] != "watchdog" {
		t.Fatalf("event = %#v", event)
	}
	for key, want := range map[string]float64{"active_card_images": 8, "queued": 6, "source_index": 2} {
		if event[key] != want {
			t.Fatalf("%s = %#v, want %v（event=%#v）", key, event[key], want, event)
		}
	}
	if event["image_source"] != "gtimg" {
		t.Fatalf("image_source = %#v", event["image_source"])
	}
}

// 放宽只针对这一条事件：watchdog 是 card_image_stalled 唯一的 reason，其它
// reason 仍要被拒并留下 client_diagnostic_rejected，否则前端写错字都看不出来。
func TestR130CardImageStalledIsWhitelisted(t *testing.T) {
	reasons, knownEvent := clientDiagnosticEvents["card_image_stalled"]
	if !knownEvent {
		t.Fatal("card_image_stalled 不在白名单里：卡片图名额泄漏会再次变成盲区")
	}
	if !reasons["watchdog"] {
		t.Fatalf("reasons = %#v, want 含 watchdog", reasons)
	}
	if len(reasons) != 1 {
		t.Fatalf("reasons = %#v, want 只有 watchdog 一条", reasons)
	}
	if reasons["other"] {
		t.Fatalf("未知 reason 竟然被放行: %#v", reasons)
	}

	instance := newR127DiagnosticApp(t)
	if code := r127PostDiagnostic(t, instance, `{"event":"card_image_stalled","reason":"other"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown reason status = %d, want 400", code)
	}
	events := r127DiagnosticEvents(t, instance)
	if len(events) != 1 || events[0]["event"] != "client_diagnostic_rejected" || events[0]["reason"] != "unknown-reason" {
		t.Fatalf("events = %#v", events)
	}
}

// 计数必须夹紧、来源必须过枚举：前端拿到的是页面实时状态，异常值不能原样进日志，
// 枚举外的 imageSource 要整个键丢掉，而不是「截断后保留」。
func TestR130CardImageStalledClampsAndDropsUnsafeValues(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantNumbers     map[string]float64
		wantImageSource string // 空串表示 image_source 这个键必须根本不出现
	}{
		{
			name:        "越界值夹到上限",
			body:        `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":999999,"queued":99999999,"sourceIndex":99999}`,
			wantNumbers: map[string]float64{"active_card_images": 1000, "queued": 100000, "source_index": 100},
		},
		{
			name:        "负值夹到 0",
			body:        `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":-5,"queued":-1,"sourceIndex":-9}`,
			wantNumbers: map[string]float64{"active_card_images": 0, "queued": 0, "source_index": 0},
		},
		{
			name:            "枚举内的来源类别照记",
			body:            `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":3,"queued":1,"sourceIndex":0,"imageSource":"communitydragon"}`,
			wantNumbers:     map[string]float64{"active_card_images": 3, "queued": 1, "source_index": 0},
			wantImageSource: "communitydragon",
		},
		{
			name:        "枚举外的来源整个键丢弃",
			body:        `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":3,"queued":1,"sourceIndex":0,"imageSource":"tencent-cdn"}`,
			wantNumbers: map[string]float64{"active_card_images": 3, "queued": 1, "source_index": 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instance := newR127DiagnosticApp(t)
			if code := r127PostDiagnostic(t, instance, test.body); code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", code)
			}
			events := r127DiagnosticEvents(t, instance)
			r127AssertNoRejection(t, events)
			if len(events) != 1 {
				t.Fatalf("events = %#v", events)
			}
			event := events[0]
			if event["event"] != "card_image_stalled" || event["reason"] != "watchdog" {
				t.Fatalf("event = %#v", event)
			}
			for key, want := range test.wantNumbers {
				if event[key] != want {
					t.Fatalf("%s = %#v, want %v（event=%#v）", key, event[key], want, event)
				}
			}
			if test.wantImageSource == "" {
				if _, ok := event["image_source"]; ok {
					t.Fatalf("未列入枚举的 imageSource 被原样记录: %#v", event)
				}
				return
			}
			if event["image_source"] != test.wantImageSource {
				t.Fatalf("image_source = %#v, want %q", event["image_source"], test.wantImageSource)
			}
		})
	}
}

// 隐私红线：即使前端把整条资源路径塞进 imageSource，落盘的也只能是枚举值，
// 路径必须一个字都不剩。
func TestR130CardImageStalledNeverLeaksResourcePaths(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"card_image_stalled","reason":"watchdog","activeCardImages":12,"queued":34,` +
		`"sourceIndex":1,"imageSource":"/api/image?path=/lol-game-data/assets/v1/profile-icons/4379.jpg"}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if _, ok := events[0]["image_source"]; ok {
		t.Fatalf("未列入枚举的 imageSource 被原样记录: %#v", events[0])
	}
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		for _, leaked := range []string{"profile-icons", "4379", "/api/image", "lol-game-data"} {
			if strings.Contains(string(encoded), leaked) {
				t.Fatalf("诊断泄漏了 %q: %s", leaked, encoded)
			}
		}
	}
}

// 新字段挂在共享的 clientDiagnosticRequest 上，所以别的上报带着它们也必须照收，
// 但只有 card_image_stalled 能把它们写进日志——这是 R127 对图片计时字段钉过的
// 同一条性质：字段不许串到别的事件上。
func TestR130CardImageStalledFieldsStayScopedToThatEvent(t *testing.T) {
	instance := newR127DiagnosticApp(t)
	body := `{"event":"local_request_client","reason":"failed","endpoint":"image","errorKind":"timeout",` +
		`"activeCardImages":8,"queued":6,"sourceIndex":2}`
	if code := r127PostDiagnostic(t, instance, body); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204：共享请求体上的新字段不该让别的上报被拒收", code)
	}
	events := r127DiagnosticEvents(t, instance)
	r127AssertNoRejection(t, events)
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events[0]
	if event["event"] != "local_request_client" || event["reason"] != "failed" || event["endpoint"] != "image" {
		t.Fatalf("event = %#v", event)
	}
	for _, key := range []string{"active_card_images", "queued", "source_index"} {
		if _, ok := event[key]; ok {
			t.Fatalf("卡片图字段串到了 local_request_client：%s = %#v（event=%#v）", key, event[key], event)
		}
	}
}
