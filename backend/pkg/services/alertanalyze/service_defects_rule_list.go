package alertanalyze

import (
	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
)

func (s *service) GetDefectDetectRuleList(req *request.GetDefectDetectRuleListRequest) *response.GetDefectDetectRuleListResponse {
	filter := &request.AlertRuleFilter{
		Group: "异常检测",
	}
	rules, totalCount := s.k8sRepo.GetAlertRules("", filter, req.PageParam, false)
	resp := &response.GetDefectDetectRuleListResponse{
		AlertRule: rules,
		Pagination: model.Pagination{
			Total:       int64(totalCount),
			CurrentPage: req.CurrentPage,
			PageSize:    req.PageSize,
		},
	}
	return resp
}
