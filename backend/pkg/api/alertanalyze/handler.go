package alertanalyze

import (
	"github.com/CloudDetail/apo/backend/pkg/core"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
	"github.com/CloudDetail/apo/backend/pkg/repository/database"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
	"github.com/CloudDetail/apo/backend/pkg/services/alertanalyze"
	"github.com/CloudDetail/apo/backend/pkg/services/serviceoverview"
	"go.uber.org/zap"
)

type Handler interface {
	// ========================告警分析========================

	// GetAlertImpact 获取告警数据的影响面
	// @Tags API.alerts
	// @Router /api/alerts/event/impact [get]
	GetAlertImpact() core.HandlerFunc

	// GetDescendantAnormalEvent 获取下游告警事件
	// @Tags API.alerts
	// @Router /api/alerts/descendant/anormal [get]
	GetDescendantAnormalEvent() core.HandlerFunc

	// GetAnomalySpan 获取服务和根因类型的故障报告
	// @Tags API.alert
	// @Router /api/service/anomaly-span/list [post]
	GetAnomalySpan() core.HandlerFunc
}

type handler struct {
	logger                 *zap.Logger
	alertanalyzeService    alertanalyze.Service
	serviceoverviewService serviceoverview.Service
}

func New(logger *zap.Logger, chRepo clickhouse.Repo, dbRepo database.Repo, promRepo prometheus.Repo) Handler {
	return &handler{
		logger:                 logger,
		alertanalyzeService:    alertanalyze.New(chRepo, promRepo),
		serviceoverviewService: serviceoverview.New(chRepo, dbRepo, promRepo),
	}
}
