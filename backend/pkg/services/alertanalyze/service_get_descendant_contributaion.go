package alertanalyze

import (
	"time"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/polarisanalyzer"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

// GetDescendantContribution 基于入口获取下游的故障贡献度排序
func (s *service) GetDescendantContribution(req *request.GetDescendantAlertContributaionRequest) (resp *response.GetDescendantAlertContributationResponse, err error) {
	// 查询所有子孙节点
	nodes, err := s.chRepo.ListDescendantNodes(&req.GetDescendantMetricsRequest)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return &response.GetDescendantAlertContributationResponse{
			LatencyContributationList: make([]model.EndpointKey, 0),
		}, nil
	}

	unsortedDescendant := make([]polarisanalyzer.LatencyRelevance, 0, len(nodes))
	for _, node := range nodes {
		unsortedDescendant = append(unsortedDescendant, polarisanalyzer.LatencyRelevance{
			Service:  node.Service,
			Endpoint: node.Endpoint,
		})
	}

	// 按延时相似度排序
	sortResp, err := s.polRepo.SortDescendantByLatencyRelevance(
		req.StartTime, req.EndTime, prometheus.VecFromDuration(time.Duration(req.Step)*time.Microsecond),
		req.Service, req.Endpoint,
		unsortedDescendant,
	)

	if err != nil {
		return &response.GetDescendantAlertContributationResponse{}, err
	}

	var latencyContributationList = []model.EndpointKey{}
	for i := 0; i < len(sortResp.SortedDescendant) && i < 3; i++ {
		latencyContributationList = append(latencyContributationList, model.EndpointKey{
			ServiceName: sortResp.SortedDescendant[i].Service,
			Endpoint:    sortResp.SortedDescendant[i].Endpoint,
		})
	}
	return &response.GetDescendantAlertContributationResponse{
		LatencyContributationList: latencyContributationList,
	}, nil
}
