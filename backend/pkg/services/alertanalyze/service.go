package alertanalyze

import (
	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
	"github.com/CloudDetail/apo/backend/pkg/repository/kubernetes"
	"github.com/CloudDetail/apo/backend/pkg/repository/polarisanalyzer"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

var _ Service = (*service)(nil)

type Service interface {
	// ========================告警分析========================

	// AlertImpact 获取告警事件的影响面
	// 如果关联所需的Label不足,error会返回ErrAlertImpactMissingTag提示期望哪些tag
	AlertImpact(req *request.AlertImpactRequest) ([]clickhouse.EntryNodeRelations, []response.ImpactAlertEvent, map[model.EndpointKey]int, *model.Pagination, error)

	// 获取下游的故障贡献度排序
	GetDescendantContribution(req *request.GetDescendantAlertContributaionRequest) (resp *response.GetDescendantAlertContributationResponse, err error)

	// GetAnomalySpan 获取可分析的异常Span
	GetAnomalySpan(req *request.GetAnomalySpanRequest) (response.GetAnomalySpanResponse, error)

	// 获取节点的所有下游节点的异常
	GetDescendantAnormal(req *request.GetDescendantAnormalEventRequest) (*response.GetDescendantAnormalEventResponse, error)

	// SearchAnormalDeltaByEntry 获取节点的所有下游节点的异常趋势
	SearchAnormalDeltaByEntry(req *request.GetDescendantAnormalDeltaEventRequest) (*response.GetDescendantDeltaAnormalEventResponse, error)

	// 获取预定义的检测表达式
	DetectDefectsOptions() *response.GetPredefinedDetectExprResponse

	// 缺陷检测
	DetectDefects(req *request.DetectMutationRequest) error
}

type service struct {
	chRepo   clickhouse.Repo
	promRepo prometheus.Repo
	polRepo  polarisanalyzer.Repo

	k8sRepo kubernetes.Repo
}

func New(chRepo clickhouse.Repo, promRepo prometheus.Repo, polRepo polarisanalyzer.Repo) Service {
	return &service{
		chRepo:   chRepo,
		promRepo: promRepo,
		polRepo:  polRepo,
	}
}
