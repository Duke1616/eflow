package channel

import (
	"github.com/Duke1616/eflow/internal/pkg/notification/provider"
	"github.com/gotomicro/ego/core/elog"
)

type wechatChannel struct {
	baseChannel
}

func NewWechatChannel(builder provider.SelectorBuilder) Channel {
	return &wechatChannel{
		baseChannel{
			builder: builder,
			logger:  elog.DefaultLogger,
		},
	}
}
