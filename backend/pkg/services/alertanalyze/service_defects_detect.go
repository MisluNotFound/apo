package alertanalyze

import (
	"context"
	"fmt"
	"time"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

// DetectDefects 缺陷检测 查询当前输入时间段内的缺陷,
func (s *service) DetectDefects(req *request.DetectMutationRequest) error {
	// TODO 按分钟取整
	startTime := time.UnixMicro(req.StartTime)
	endTime := time.UnixMicro(req.EndTime)
	step := time.Duration(req.Step * int64(time.Microsecond))

	if len(req.MutataionCheck.Group) == 0 {
		req.MutataionCheck.Group = "mutation-custom"
	}

	// 准备告警事件所需的Detail模版
	pql := req.MutataionCheck.String()
	upperLimit := req.UpperLimit.String()
	lowerLimit := req.LowerLimit.String()
	var summary string
	if req.UpperLimit != nil || req.LowerLimit != nil {
		summary = fmt.Sprintf("(%s) exceeds the set threshold (lower: %s,upper: %s)", req.MutataionCheck.Metric, lowerLimit, upperLimit)
	} else {
		summary = fmt.Sprintf("(%s) detected", &req.MutataionCheck)
	}

	mutationPQLCheck := &prometheus.MutationPQLCheck{
		PQL:        pql,
		UpperLimit: upperLimit,
		LowerLimit: lowerLimit,
	}

	if req.SynchronizeToAlertRules {
		err := s.k8sRepo.AddAlertRule("", request.AlertRule{
			Group:  req.MutataionCheck.Group,
			Alert:  req.DetectName,
			Expr:   mutationPQLCheck.GetExecutedPQL(),
			For:    req.For,
			Labels: map[string]string{},
			Annotations: map[string]string{
				"description": req.DetectName + " mutation detected\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}",
				"summary":     summary,
			},
		})

		if err != nil {
			// Deal with error
		}
	}

	metricResults, err := s.promRepo.ExecutedMutationCheck(mutationPQLCheck, startTime, endTime, step)
	if err != nil {
		// TODO deal err
	}

	var forDurationNS int64 = 0
	if len(req.For) > 0 {
		duration, err := time.ParseDuration(req.For)
		if err != nil {
			// TODO deal err
		}
		forDurationNS = duration.Nanoseconds()
	}

	var totalAlertEvents []*model.AlertEvent

	for _, metricResult := range metricResults {
		points := metricResult.Values
		mutationRanges := splitContinuePoint(points, forDurationNS)
		for i := 0; i < len(mutationRanges); i++ {
			alertEvents := transferMetricResult2AlertEvent(
				req.DetectName, req.MutataionCheck.Group,
				req.EndTime, req.Step,
				forDurationNS, summary,
				metricResult.Metric,
				mutationRanges[i],
			)

			totalAlertEvents = append(totalAlertEvents, alertEvents...)
		}
	}

	// 将时间周期内生成的告警事件存入Clickhouse
	return s.chRepo.InsertBatchAlertEvents(context.Background(), totalAlertEvents)
}

// splitContinuePoint 将时间范围内的所有点,按是否连续分隔成多段
func splitContinuePoint(points []prometheus.Points, forDurationNS int64) [][]prometheus.Points {
	if len(points) == 0 {
		return [][]prometheus.Points{}
	}

	var res [][]prometheus.Points
	var lastSeenTime int64
	var tmpGroup []prometheus.Points = []prometheus.Points{points[0]}
	for i := 1; i < len(points); i++ {
		// 距离上次告警时间超过了For时间,中断这次告警
		if points[i].TimeStamp-lastSeenTime > forDurationNS/1e9 {
			// 非连续
			res = append(res, tmpGroup)
			tmpGroup = []prometheus.Points{points[i]}
			lastSeenTime = points[i].TimeStamp
		} else {
			// 连续
			tmpGroup = append(tmpGroup, points[i])
		}
	}

	return res
}

func transferMetricResult2AlertEvent(
	detectName string, group string,
	endTimeMicroSecond int64,
	stepMicroSecond int64,
	forDurationNS int64,
	summary string,
	labels prometheus.Labels,
	mutationRange []prometheus.Points,
) []*model.AlertEvent {
	if len(mutationRange) == 0 {
		return nil
	}
	rangeStart := mutationRange[0].TimeStamp
	rangeEnd := mutationRange[len(mutationRange)-1].TimeStamp

	if rangeEnd-rangeStart < forDurationNS/1e9 {
		// 不生成告警事件
		return nil
	}

	tags := extractTagsFromLabels(detectName, labels)
	detailTemplate := extractDetailTemplate(detectName, tags, summary)

	// 为故障事件范围内所有超过ForDuration的点生成事件和更新事件
	var alertEvents []*model.AlertEvent
	var noEnd time.Time = time.Unix(0, 0)
	alertStart := rangeStart + forDurationNS/1e9
	for _, point := range mutationRange {
		if point.TimeStamp > alertStart {
			alertEvents = append(alertEvents, &model.AlertEvent{
				ID:           model.GenUUID(),
				Source:       "apo-backend",
				CreateTime:   time.Unix(alertStart, 0),
				UpdateTime:   time.Unix(point.TimeStamp, 0),
				EndTime:      noEnd,
				ReceivedTime: time.Unix(point.TimeStamp, 0),
				Severity:     model.SeverityLevelWarning,
				Group:        group,
				Name:         detectName,
				Detail:       fmt.Sprintf(detailTemplate, point.Value),
				Tags:         tags,
				Status:       model.StatusFiring,
			})
		}
	}

	if len(alertEvents) > 0 {
		lastPoint := mutationRange[len(mutationRange)-1]
		shouldResolvedTime := lastPoint.TimeStamp + stepMicroSecond/1e6
		// 如果查询时间端包含了告警区间的后一个点
		if shouldResolvedTime <= endTimeMicroSecond {
			alertEvents = append(alertEvents, &model.AlertEvent{
				ID:           model.GenUUID(),
				Source:       "apo-backend",
				CreateTime:   time.Unix(alertStart, 0),
				UpdateTime:   time.Unix(shouldResolvedTime, 0),
				EndTime:      noEnd,
				ReceivedTime: time.Unix(shouldResolvedTime, 0),
				Severity:     model.SeverityLevelInfo,
				Group:        group,
				Name:         detectName,
				Detail:       extractResolvedDetail(detectName, tags, summary),
				Tags:         tags,
				Status:       model.StatusResolved,
			})
		}
	}

	return alertEvents
}

func extractTagsFromLabels(name string, labels prometheus.Labels) map[string]string {
	var res = map[string]string{}
	res["name"] = name

	if len(labels.ContainerID) > 0 {
		res["container_id"] = labels.ContainerID
	}

	if len(labels.ContentKey) > 0 {
		res["content_key"] = labels.ContentKey
	}

	if len(labels.Instance) > 0 {
		res["instance"] = labels.Instance
	}

	if len(labels.IsError) > 0 {
		res["is_error"] = labels.IsError
	}

	if len(labels.Job) > 0 {
		res["job"] = labels.Job
	}

	if len(labels.NodeName) > 0 {
		res["node_name"] = labels.NodeName
	}

	if len(labels.POD) > 0 {
		res["pod"] = labels.POD
	}

	if len(labels.SvcName) > 0 {
		res["svc_name"] = labels.SvcName
	}

	if len(labels.TopSpan) > 0 {
		res["top_span"] = labels.TopSpan
	}

	if len(labels.PID) > 0 {
		res["pid"] = labels.PID
	}

	if len(labels.PodName) > 0 {
		res["pod_name"] = labels.PodName
	}

	if len(labels.Namespace) > 0 {
		res["namespace"] = labels.Namespace
	}

	if len(labels.NodeIP) > 0 {
		res["node_ip"] = labels.NodeIP
	}

	if len(labels.DBSystem) > 0 {
		res["db_system"] = labels.DBSystem
	}

	if len(labels.DBName) > 0 {
		res["db_name"] = labels.DBName
	}

	if len(labels.DBUrl) > 0 {
		res["db_url"] = labels.DBUrl
	}

	if len(labels.MonitorName) > 0 {
		res["monitor_name"] = labels.MonitorName
	}

	return res
}

func extractDetailTemplate(name string, tags map[string]string, summary string) string {
	tagsStr := fmt.Sprintf("%v", tags)
	return `{"description":"` + name + `"\n  VALUE = %f\n  LABELS = ` + tagsStr + `","summary":"` + summary + `"}`
}

func extractResolvedDetail(name string, tags map[string]string, summary string) string {
	tagsStr := fmt.Sprintf("%v", tags)
	return `{"description":"` + name + `"\n  LABELS = ` + tagsStr + `","summary":"` + summary + `"}`
}
