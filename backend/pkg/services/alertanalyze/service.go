package alertanalyze

import (
	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

var _ Service = (*service)(nil)

type Service interface {
	// ========================告警分析========================

	// AlertImpact 获取告警事件的影响面
	// 如果关联所需的Label不足,error会返回ErrAlertImpactMissingTag提示期望哪些tag
	AlertImpact(req *request.AlertImpactRequest) ([]clickhouse.EntryNodeRelations, []response.ImpactAlertEvent, *model.Pagination, error)

	// SearchAnormalEventByEntry 查询入口节点下游的异常事件
	SearchAnormalEventByEntry(req *request.GetDescendantAnormalEventRequest) (*response.GetDescendantAnormalEventResponse, error)

	// GetAnomalySpan 获取可分析的异常Span
	GetAnomalySpan(req *request.GetAnomalySpanRequest) (response.GetAnomalySpanResponse, error)
}

type service struct {
	chRepo   clickhouse.Repo
	promRepo prometheus.Repo
}

func New(chRepo clickhouse.Repo, promRepo prometheus.Repo) Service {
	return &service{
		chRepo:   chRepo,
		promRepo: promRepo,
	}
}
