package render

import (
	"context"
	"errors"
)

// ErrNotReady 表示动态渲染入口还没有实现。
var ErrNotReady = errors.New("动态渲染尚未实现")

// Request 是一次隔离渲染要打开的 Banner HTML。
type Request struct {
	HTML          string
	Width, Height int
}

// Snapshot 是渲染后的截图、文案和资源。当前入口不产出这些内容。
type Snapshot struct {
	Screenshot []byte
	Texts      []string
	Failure    string
}

// Render 保留动态素材的调用入口。素材获取现在不调用它。
func Render(ctx context.Context, req Request) (Snapshot, error) {
	return Snapshot{Failure: "动态渲染尚未实现"}, ErrNotReady
}
