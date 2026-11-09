package errs

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKratosStyleErrors(t *testing.T) {
	testCases := []struct {
		name         string
		err          *Error
		wantCode     int32
		wantReason   string
		wantMessage  string
		wantIsBadReq bool
	}{
		{
			name:         "BadRequest 构造与断言",
			err:          BadRequest("PARAM_INVALID", "驳回原因不能为空"),
			wantCode:     http.StatusBadRequest,
			wantReason:   "PARAM_INVALID",
			wantMessage:  "驳回原因不能为空",
			wantIsBadReq: true,
		},
		{
			name:         "Forbidden 构造与断言",
			err:          Forbidden("PERMISSION_DENIED", "无权审批该任务"),
			wantCode:     http.StatusForbidden,
			wantReason:   "PERMISSION_DENIED",
			wantMessage:  "无权审批该任务",
			wantIsBadReq: false,
		},
		{
			name:         "NotFound 构造与断言",
			err:          NotFound("TICKET_NOT_FOUND", "工单不存在"),
			wantCode:     http.StatusNotFound,
			wantReason:   "TICKET_NOT_FOUND",
			wantMessage:  "工单不存在",
			wantIsBadReq: false,
		},
		{
			name:         "Conflict 构造与断言",
			err:          Conflict("TASK_ALREADY_FINISHED", "任务已被他人处理"),
			wantCode:     http.StatusConflict,
			wantReason:   "TASK_ALREADY_FINISHED",
			wantMessage:  "任务已被他人处理",
			wantIsBadReq: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantCode, tc.err.Code)
			assert.Equal(t, tc.wantReason, tc.err.Reason)
			assert.Equal(t, tc.wantMessage, tc.err.Message)
			assert.Equal(t, tc.wantIsBadReq, IsBadRequest(tc.err))
			assert.True(t, IsReason(tc.err, tc.wantReason))
			assert.Equal(t, tc.wantReason, Reason(tc.err))
		})
	}
}

func TestErrorWithCauseAndUnwrap(t *testing.T) {
	rootCause := errors.New("db connection timeout")
	bizErr := BadRequest("DB_QUERY_FAILED", "查询工单失败").WithCause(rootCause)

	assert.True(t, errors.Is(bizErr, rootCause))
	assert.Equal(t, rootCause, bizErr.Unwrap())
	assert.Contains(t, bizErr.Error(), "db connection timeout")

	// 测试元数据链式注入
	bizErr.WithMetadata(map[string]string{"ticket_id": "1001"})
	assert.Equal(t, "1001", bizErr.Metadata["ticket_id"])
}

func TestErrorIsByReason(t *testing.T) {
	err1 := BadRequest("TICKET_NOT_FOUND", "工单未找到 (模块A)")
	err2 := NotFound("TICKET_NOT_FOUND", "工单未找到 (模块B)")
	other := BadRequest("OTHER_REASON", "其他错误")

	// 只要 Reason 相同，即被判定为匹配（克服指针不相等问题）
	assert.True(t, err1.Is(err2))
	assert.False(t, err1.Is(other))
}

func TestCodeAndMessageHelper(t *testing.T) {
	t.Run("标准业务错误提取 Code 与 Message", func(t *testing.T) {
		err := BadRequest("COMMENT_EMPTY", "驳回原因不能为空")
		assert.Equal(t, int32(http.StatusBadRequest), Code(err))
		assert.Equal(t, "驳回原因不能为空", Message(err))
	})

	t.Run("包含包装的业务错误同样能正确提取", func(t *testing.T) {
		err := fmt.Errorf("service layer wrap: %w", Forbidden("NO_AUTH", "无权访问"))
		assert.Equal(t, int32(http.StatusForbidden), Code(err))
		assert.Equal(t, "无权访问", Message(err))
	})

	t.Run("未知名裸错误兜底为 500", func(t *testing.T) {
		rawErr := errors.New("unexpected nil pointer")
		assert.Equal(t, int32(http.StatusInternalServerError), Code(rawErr))
		assert.Equal(t, "系统内部错误", Message(rawErr))
	})

	t.Run("nil 错误返回 200 和空信息", func(t *testing.T) {
		assert.Equal(t, int32(http.StatusOK), Code(nil))
		assert.Equal(t, "", Message(nil))
	})
}
