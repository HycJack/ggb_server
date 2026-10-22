package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestBoardSnapshotFromBridge 页面回传 data URL 后，Go 应能解出位图。
func TestBoardSnapshotFromBridge(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 8), 200, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	a := &app{msgsOf: map[uint][]*uiMsg{}, boardPending: map[string]chan boardResult{}}
	bridge := &GGBBridge{a}

	bridge.Image(context.Background(), "b1", dataURL)

	if a.boardImageURL != dataURL {
		t.Errorf("boardImageURL 未保存")
	}
	bm := a.boardBitmap()
	if bm == nil {
		t.Fatalf("boardBitmap() = nil，截图没解出来")
	}
	if a.boardBitmapCache != bm {
		t.Errorf("位图没有缓存住")
	}
}

// TestSnapshotDeliveredToWaiter op=image 的同步调用也应拿到 data URL。
func TestSnapshotDeliveredToWaiter(t *testing.T) {
	a := &app{msgsOf: map[uint][]*uiMsg{}, boardPending: map[string]chan boardResult{}}
	ch := make(chan boardResult, 1)
	a.boardPending["x"] = ch
	if !a.deliverBoard("x", boardResult{image: "data:image/png;base64,AAA"}) {
		t.Errorf("deliverBoard 未送达")
	}
	select {
	case r := <-ch:
		if r.image == "" {
			t.Errorf("图片内容为空")
		}
	default:
		t.Errorf("等待者没收到结果")
	}
}
