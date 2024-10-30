package alertanalyze

import (
	"github.com/CloudDetail/apo/backend/pkg/core"
)

// GetPredefinedDetectExpr 获取预定义检测表达式
// @Summary 获取预定义检测表达式
// @Description 获取预定义检测表达式
// @Tags API.alerts
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Success 200 {object} response.GetPredefinedDetectExprResponse
// @Failure 400 {object} code.Failure
// @Router /api/alerts/detect/mutation/metrics [get]
func (h *handler) GetPredefinedDetectExpr() core.HandlerFunc {
	return func(c core.Context) {
		resp := h.alertanalyzeService.DetectDefectsOptions()
		c.Payload(resp)
	}
}
