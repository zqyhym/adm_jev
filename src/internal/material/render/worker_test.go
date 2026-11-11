package render

import (
	"context"
	"errors"
	"testing"
)

// TestRenderNotReady 验证动态渲染入口返回未实现，不产出截图。
func TestRenderNotReady(t *testing.T) {
	got, err := Render(context.Background(), Request{HTML: "<div></div>", Width: 320, Height: 50})
	if !errors.Is(err, ErrNotReady) || got.Failure == "" || len(got.Screenshot) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
