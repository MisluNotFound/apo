package alertanalyze

import (
	"time"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
)

// 查询入口节点的下游告警情况
func (s *service) GetDescendantAnormal(req *request.GetDescendantAnormalEventRequest) (*response.GetDescendantAnormalEventResponse, error) {
	startTime := time.UnixMicro(req.StartTime)
	endTime := time.UnixMicro(req.EndTime)

	if req.PageParam == nil {
		req.PageParam = &request.PageParam{
			CurrentPage: 1,
			PageSize:    999,
		}
	}

	// 先查询入口节点的下游节点
	nodes, err := s.chRepo.ListDescendantNodes(&req.GetDescendantMetricsRequest)
	if err != nil {
		return &response.GetDescendantAnormalEventResponse{}, err
	}

	var instances []*model.ServiceInstance
	tmpServiceSet := map[string]struct{}{}
	endpointList := make([]model.EndpointKey, 0)

	for _, node := range nodes {
		endpointList = append(endpointList, model.EndpointKey{
			ServiceName: node.Service,
			ContentKey:  node.Endpoint,
		})

		if _, find := tmpServiceSet[node.Service]; find {
			continue
		}
		tmpServiceSet[node.Service] = struct{}{}

		// 忽略URL查询Instance
		instanceList, err := s.promRepo.GetInstanceList(req.StartTime, req.EndTime, node.Service, "")
		if err != nil {
			continue
		}
		instances = append(instances, instanceList.GetInstances()...)
	}

	// 通过下游节点查询告警事件
	events, count, err := s.chRepo.GetAlertEventsByInstanceAndEndpoints(startTime, endTime,
		request.AlertFilter{Status: "firing"},
		instances, endpointList, req.PageParam,
		clickhouse.ReceivedTimeOrders,
	)

	// TODO 告警事件关联到影响服务
	return &response.GetDescendantAnormalEventResponse{
		AlertEvents: events,
		Pagination: &model.Pagination{
			Total:       int64(count),
			CurrentPage: req.PageParam.CurrentPage,
			PageSize:    req.PageParam.PageSize,
		},
	}, err
}
