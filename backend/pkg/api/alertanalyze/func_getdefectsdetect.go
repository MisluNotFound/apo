package alertanalyze

import (
	"net/http"

	"github.com/CloudDetail/apo/backend/pkg/code"
	"github.com/CloudDetail/apo/backend/pkg/core"

	"github.com/CloudDetail/apo/backend/pkg/model/request"
)

// TODO 迁移到 model/request包中
type getDefectsDetectRequest struct {
}

// TODO 迁移到 model/response包中
type getDefectsDetectResponse struct {
}

// AddDefectsDetect 执行异常检测
// @Summary 执行异常检测
// @Description 执行异常检测
// @Tags API.alerts
// @Accept json
// @Produce json
// @Param Request body request.DetectMutationRequest true "请求信息"
// @Success 200
// @Failure 400 {object} code.Failure
// @Router /api/alerts/detect/mutation/add [post]
func (h *handler) AddDefectsDetect() core.HandlerFunc {
	return func(c core.Context) {
		req := new(request.DetectMutationRequest)
		if err := c.ShouldBindJSON(req); err != nil {
			c.AbortWithError(core.Error(
				http.StatusBadRequest,
				code.ParamBindError,
				code.Text(code.ParamBindError)).WithError(err),
			)
			return
		}

		err := h.alertanalyzeService.DetectDefects(req)
		if err != nil {
			c.AbortWithError(core.Error(
				http.StatusBadRequest,
				code.DetectDefectsError,
				code.Text(code.DetectDefectsError)).WithError(err),
			)
			return
		}
		c.Payload("ok")
	}
}
