package alertanalyze

import (
	"errors"
	"net/http"
	"time"

	"github.com/CloudDetail/apo/backend/pkg/code"
	"github.com/CloudDetail/apo/backend/pkg/core"
	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
	"github.com/CloudDetail/apo/backend/pkg/services/serviceoverview"
)

// GetAlertImpact 获取告警数据的影响面
// @Summary 获取告警数据的影响面
// @Description 获取告警数据的影响面
// @Tags API.alerts
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param eventId query string false "查询告警事件ID"
// @Param startTime query uint64 true "查询开始时间"
// @Param endTime query uint64 true "查询结束时间"
// @Param step query int64 true "查询步长(us)"
// @Success 200 {object} []response.GetServiceEntryEndpointsResponse
// @Failure 400 {object} code.Failure
// @Router /api/alerts/event/impact [get]
func (h *handler) GetAlertImpact() core.HandlerFunc {
	return func(c core.Context) {
		req := new(request.AlertImpactRequest)
		if err := c.ShouldBindQuery(req); err != nil {
			c.AbortWithError(core.Error(
				http.StatusBadRequest,
				code.ParamBindError,
				code.Text(code.ParamBindError)).WithError(err),
			)
			return
		}

		entryNodes, events, pagation, err := h.alertanalyzeService.AlertImpact(req)
		if err != nil {
			var vErr model.ErrAlertImpactMissingTag
			var vErr2 model.ErrAlertImpactNoMatchedService
			if errors.As(err, &vErr) {
				// 告警所需的关联信息不足
				// 提示告警中需要包含下列任意label组合
				c.AbortWithError(core.Error(
					http.StatusBadRequest,
					code.AlertEventImpactMissingTag,
					code.Text(code.AlertEventImpactMissingTag)+vErr.CheckedTagGroups()).WithError(err),
				)
				return
			} else if errors.As(err, &vErr2) {
				c.AbortWithError(core.Error(
					http.StatusBadRequest,
					code.AlertEventImpactNoMatchedService,
					code.Text(code.AlertEventImpactNoMatchedService)+vErr2.CheckedTagGroup()).WithError(err),
				)
				return
			} else {
				// 查询失败
				c.AbortWithError(core.Error(
					http.StatusBadRequest,
					code.AlertEventImpactError,
					code.Text(code.AlertEventImpactError)).WithError(err),
				)
				return
			}
		}

		// 填充EntryEndpoint信息
		resp, err := h.FillEntryNodeDetail(req, entryNodes)
		if err != nil {
			// 查询失败
			c.AbortWithError(core.Error(
				http.StatusBadRequest,
				code.AlertEventImpactError,
				code.Text(code.AlertEventImpactError)).WithError(err),
			)
			return
		}
		resp.AlertEvents = events
		resp.Pagination = pagation
		c.Payload(resp)
	}
}

// FillEntryNodeDetail 填充EntryEndpoint信息
// 复制于 backend/pkg/api/service/func_getserviceentryendpoints.go
func (h *handler) FillEntryNodeDetail(req *request.AlertImpactRequest, entryNodes []clickhouse.EntryNodeRelations) (*response.GetAlertImpactResponse, error) {
	result := make(map[string]*response.EntryInstanceData, 0)
	resp := response.GetAlertImpactResponse{
		Data: make([]*response.EntryInstanceData, 0),
	}

	// threshold, err := h.serviceoverviewService.GetThreshold(database.GLOBAL, "", "")
	// if err != nil {
	// 	// 获取全局Threshold失败，使用默认值
	// 	threshold = response.GetThresholdResponse{
	// 		Latency:   5,
	// 		ErrorRate: 5,
	// 		Tps:       5,
	// 		Log:       5,
	// 	}
	// }

	startTime := time.UnixMicro(req.StartTime)
	endTime := time.UnixMicro(req.EndTime)
	sortRule := serviceoverview.DODThreshold
	step := time.Duration(req.Step * 1000)

	endpoints := make([]prometheus.EndpointKey, 0)
	for _, entryNode := range entryNodes {
		endpoints = append(endpoints, prometheus.EndpointKey{SvcName: entryNode.Service, ContentKey: entryNode.Endpoint})
	}

	endpointResps, err := h.serviceoverviewService.GetServicesEndpointDataByEndpoints(startTime, endTime, step, endpoints, sortRule)
	if err != nil {
		return nil, err
	}

	for _, endpointResp := range endpointResps {
		if serviceResp, found := result[endpointResp.ServiceName]; found {
			serviceResp.Namespaces = endpointResp.Namespaces
			serviceResp.EndpointCount += endpointResp.EndpointCount
			serviceResp.AddNamespaces(endpointResp.Namespaces)
		} else {
			result[endpointResp.ServiceName] = &response.EntryInstanceData{
				ServiceName:    endpointResp.ServiceName,
				Namespaces:     endpointResp.Namespaces,
				EndpointCount:  endpointResp.EndpointCount,
				ServiceDetails: endpointResp.ServiceDetails,
			}
		}

		// for _, detail := range endpointResp.ServiceDetails {
		// 	if detail.Latency.Ratio.DayOverDay != nil && *detail.Latency.Ratio.DayOverDay > threshold.Latency {
		// 		resp.Status = model.STATUS_CRITICAL
		// 	}
		// 	if detail.Latency.Ratio.WeekOverDay != nil && *detail.Latency.Ratio.WeekOverDay > threshold.Latency {
		// 		resp.Status = model.STATUS_CRITICAL
		// 	}
		// 	if detail.ErrorRate.Ratio.DayOverDay != nil && *detail.ErrorRate.Ratio.DayOverDay > threshold.ErrorRate {
		// 		resp.Status = model.STATUS_CRITICAL
		// 	}
		// 	if detail.ErrorRate.Ratio.WeekOverDay != nil && *detail.ErrorRate.Ratio.WeekOverDay > threshold.ErrorRate {
		// 		resp.Status = model.STATUS_CRITICAL
		// 	}
		// }
	}

	// serviceNames := make([]string, 0)
	// for serviceName := range result {
	// 	serviceNames = append(serviceNames, serviceName)
	// }

	// 补全日志错误数等信息
	// alertResps, err := h.serviceoverviewService.GetServicesAlert(startTime, endTime, step, serviceNames, nil)
	// if err != nil {
	// 	// 未能检查到状态,输出日志
	// 	h.logger.Error("get entryEndpoint alert error", zap.Error(err))
	// }
	// for _, alertResp := range alertResps {
	// 	if serviceResp, found := result[alertResp.ServiceName]; found {
	// 		serviceResp.Logs = alertResp.Logs
	// 		serviceResp.Timestamp = alertResp.Timestamp
	// 		serviceResp.AlertStatus = alertResp.AlertStatus
	// 		serviceResp.AlertReason = alertResp.AlertReason
	// 	}

	// 	if alertResp.Logs.Ratio.DayOverDay != nil && *alertResp.Logs.Ratio.DayOverDay > threshold.Log {
	// 		resp.Status = model.STATUS_CRITICAL
	// 	}
	// 	if alertResp.Logs.Ratio.WeekOverDay != nil && *alertResp.Logs.Ratio.WeekOverDay > threshold.Log {
	// 		resp.Status = model.STATUS_CRITICAL
	// 	}
	// 	if alertResp.AlertStatusCH.InfrastructureStatus == model.STATUS_CRITICAL ||
	// 		alertResp.AlertStatusCH.NetStatus == model.STATUS_CRITICAL ||
	// 		alertResp.AlertStatusCH.K8sStatus == model.STATUS_CRITICAL {
	// 		resp.Status = model.STATUS_CRITICAL
	// 	}
	// }

	for _, endpointsResp := range result {
		resp.Data = append(resp.Data, endpointsResp)
	}
	return &resp, nil
}
