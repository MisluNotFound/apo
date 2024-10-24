package alertanalyze

import (
	"errors"
	"fmt"
	"time"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

// AlertImpact 分析告警事件的影响面
// 1. 根据告警时间类型找到关联的Service,
// !!! 会检查Event中是否有满足要求的Label,如果没有会尝试所有预设的label组合
// 2. 通过ServiceTopology查询service的关联入口
func (s *service) AlertImpact(req *request.AlertImpactRequest) ([]clickhouse.EntryNodeRelations, []response.ImpactAlertEvent, *model.Pagination, error) {
	startTime := time.UnixMicro(req.StartTime)
	endTime := time.UnixMicro(req.EndTime)

	// 从Clickhouse中获取到所有的告警
	if req.EventID != "" {
		entrys, err := s.alertImpactTargetEvent(req.EventID, startTime, endTime)
		return entrys, nil, nil, err
	}

	// eventId为空,获取所有的告警
	events, count, err := s.chRepo.GetAlertEvents(startTime, endTime, request.AlertFilter{}, nil, nil, "")
	if err != nil {
		return nil, nil, nil, err
	}

	endpointsMap := EndpointsMap{
		EndpointSet:  map[clickhouse.AlertService]struct{}{},
		InferNodeSet: map[string][]clickhouse.AlertService{},
		PodSet:       map[podKey][]clickhouse.AlertService{},
		NetSrcSet:    map[netSrcProcessKey][]clickhouse.AlertService{},
	}

	var impactEvents = make([]response.ImpactAlertEvent, len(events))
	for i := 0; i < len(events); i++ {
		if req.AnalyzeSize == 0 && i > 50000 || req.AnalyzeSize != 0 && i > req.AnalyzeSize {
			impactEvents[i] = response.ImpactAlertEvent{
				AlertEvent:      &events[i].AlertEvent,
				IsAnalyzed:      false,
				ImpactEndpoints: []clickhouse.AlertService{},
			}
			continue
		}
		endpoints, associated := endpointsMap.SearchAndAddEndpointsForAlert(s.promRepo, &events[i].AlertEvent, startTime, endTime)
		// 记录关联状态
		impactEvents[i] = response.ImpactAlertEvent{
			AlertEvent:      &events[i].AlertEvent,
			IsAnalyzed:      associated,
			ImpactEndpoints: endpoints,
			ImpactEntrys:    []model.EndpointKey{},
		}
	}

	endpoints := endpointsMap.EndpointsList()

	// 关联入口和Endpoints
	entryRelation, err := s.chRepo.SearchEntryEndpointsByAlertService(endpoints, startTime.UnixMicro(), endTime.UnixMicro())
	if err != nil {
		pagation, eventList, _ := pagationAndFillEntry(req, count, impactEvents, nil)
		return entryRelation, eventList, pagation, err
	}

	// 分页(只做分页内的告警事件的入口关联)
	pagation, eventList, err := pagationAndFillEntry(req, count, impactEvents, entryRelation)
	return entryRelation, eventList, pagation, err
}

func pagationAndFillEntry(req *request.AlertImpactRequest, count int, impactEvents []response.ImpactAlertEvent, entryRelation []clickhouse.EntryNodeRelations) (*model.Pagination, []response.ImpactAlertEvent, error) {
	if req.PageParam == nil {
		// 按0,999分页
		req.PageParam = &request.PageParam{
			CurrentPage: 1,
			PageSize:    999,
		}
	}

	var pagation *model.Pagination = &model.Pagination{
		Total:       int64(count),
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
	}
	var from, to int
	from = (req.PageParam.CurrentPage - 1) * req.PageParam.PageSize
	to = req.PageParam.CurrentPage * req.PageParam.PageSize
	if from > len(impactEvents) {
		return pagation, nil, fmt.Errorf("page out of range")
	}
	if to > len(impactEvents) {
		to = len(impactEvents)
	}

	for i := from; i < to; i++ {
		impactEvents[i].ImpactEntrys = searchEntryRelations(entryRelation, impactEvents[i].ImpactEndpoints)
	}
	return pagation, impactEvents[from:to], nil
}

func (s *service) alertImpactTargetEvent(eventid string, startTime, endTime time.Time) ([]clickhouse.EntryNodeRelations, error) {
	event, err := s.chRepo.GetAlertEventById(eventid, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get alert eventId[%s]: %v", eventid, err)
	}

	// 根据告警事件类型查询影响入口
	var endpoints []clickhouse.AlertService
	switch clickhouse.AlertGroup(event.Group) {
	case clickhouse.APP_GROUP:
		endpoints, err = tryGetAlertServiceByService(s.promRepo, nil, event, startTime, endTime)
	case clickhouse.NETWORK_GROUP:
		// 先尝试K8sPod匹配
		endpoints, err = tryGetAlertServiceByNetK8s(s.promRepo, nil, event, startTime, endTime)
		if err != nil && errors.As(err, &model.ErrAlertImpactMissingTag{}) {
			// 再尝试Node匹配
			endpoints, err = tryGetAlertServiceByNetSrcVM(s.promRepo, nil, event, startTime, endTime)
		}
	case clickhouse.CONTAINER_GROUP:
		endpoints, err = tryGetAlertServiceByContainer(s.promRepo, nil, event, startTime, endTime)
	case clickhouse.INFRA_GROUP:
		endpoints, err = tryGetAlertServiceByInfraNode(s.promRepo, nil, event, startTime, endTime)
	}

	if err != nil && errors.As(err, &model.ErrAlertImpactMissingTag{}) {
		// 预期的Label不存在,尝试所有预设的label组合
		endpoints, err = tryGetAlertService(s.promRepo, nil, event, startTime, endTime)
	}

	if err != nil {
		return nil, err
	}

	// 通过ServiceTopology关联查询入口
	return s.chRepo.SearchEntryEndpointsByAlertService(endpoints, startTime.UnixMicro(), endTime.UnixMicro())
}

type EndpointsMap struct {
	EndpointSet map[clickhouse.AlertService]struct{}

	// Node
	InferNodeSet map[string][]clickhouse.AlertService
	// Pod Namespace
	PodSet map[podKey][]clickhouse.AlertService
	// Node + Pid
	NetSrcSet map[netSrcProcessKey][]clickhouse.AlertService

	EntryRelationsSet map[clickhouse.AlertService][]clickhouse.EntryNodeRelations
}

type podKey struct {
	Namespace string
	PodName   string
}

type netSrcProcessKey struct {
	Node string
	Pid  string
}

func (m *EndpointsMap) SearchAndAddEndpointsForAlert(repo prometheus.Repo, event *model.AlertEvent, startTime, endTime time.Time) ([]clickhouse.AlertService, bool) {
	// 尝试下面的方法去获取event的关联入口
	endpoints, err := tryGetAlertService(repo, m, event, startTime, endTime)
	if err != nil && len(endpoints) == 0 {
		var vErr model.ErrAlertImpactNoMatchedService
		if errors.As(err, &vErr) {
			// 分析过,但找不到
			return []clickhouse.AlertService{}, true
		}
		// 无法分析
		return []clickhouse.AlertService{}, false
	}

	if len(endpoints) == 0 {
		return []clickhouse.AlertService{}, true
	}
	return endpoints, true
}

func (m *EndpointsMap) EndpointsList() []clickhouse.AlertService {
	var endpointList []clickhouse.AlertService
	for endpoint := range m.EndpointSet {
		endpointList = append(endpointList, endpoint)
	}
	return endpointList
}

func searchEntryRelations(relations []clickhouse.EntryNodeRelations, endpoints []clickhouse.AlertService) []model.EndpointKey {
	var tmpEntrySet = make(map[model.EndpointKey]struct{}, 0)
	var entryList = make([]model.EndpointKey, 0)
	for _, relation := range relations {
		for _, endpoint := range endpoints {
			if relation.DescendantEndpoint != endpoint.ContentKey ||
				relation.DescendantService == endpoint.ServiceName {
				continue
			}
			entryKey := model.EndpointKey{
				ServiceName: relation.Service,
				ContentKey:  relation.Endpoint,
			}
			if _, find := tmpEntrySet[entryKey]; !find {
				continue
			}
			tmpEntrySet[entryKey] = struct{}{}
			entryList = append(entryList, entryKey)
		}
	}
	return entryList
}

func tryGetAlertService(repo prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, startTime time.Time, endTime time.Time) ([]clickhouse.AlertService, error) {
	var tryMethods = []func(prometheus.Repo, *EndpointsMap, *model.AlertEvent, time.Time, time.Time) ([]clickhouse.AlertService, error){
		tryGetAlertServiceByNetSrcVM,
		tryGetAlertServiceByContainer,
		tryGetAlertServiceByNetK8s,
		tryGetAlertServiceByService,
		tryGetAlertServiceByInfraNode,
	}
	var endpoints []clickhouse.AlertService
	checkedError := model.ErrAlertImpactMissingTag{
		TagGroups: []model.TagGroup{},
		Event:     event,
	}
	for _, tryGetService := range tryMethods {
		var err error
		endpoints, err = tryGetService(repo, endpointsMap, event, startTime, endTime)
		if err == nil {
			return endpoints, nil
		}
		// 如果是Tag不足,继续尝试别的Tag
		var vErr model.ErrAlertImpactMissingTag
		if errors.As(err, &vErr) {
			checkedError.AddCheckedGroup(vErr)
			continue
		}
		// 其他错误,直接返回
		return nil, err
	}

	return endpoints, checkedError
}

func tryGetAlertServiceByService(_ prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, _ time.Time, _ time.Time) ([]clickhouse.AlertService, error) {
	serviceName := event.GetServiceNameTag()
	if len(serviceName) == 0 {
		return nil, model.ErrAlertImpactMissingTag{
			TagGroups: []model.TagGroup{[]string{"svc_name"}},
			Event:     event,
		}
	}

	alertServices := []clickhouse.AlertService{
		{
			ServiceName: serviceName,
			ContentKey:  event.GetContentKeyTag(),
		},
	}

	if endpointsMap != nil {
		for _, alertService := range alertServices {
			endpointsMap.EndpointSet[alertService] = struct{}{}
		}
	}

	return alertServices, nil
}

func tryGetAlertServiceByNetK8s(repo prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, startTime time.Time, endTime time.Time) ([]clickhouse.AlertService, error) {
	podName := event.GetNetSrcPodTag()
	namespace := event.GetNetSrcNamespaceTag()
	if len(podName) == 0 || len(namespace) == 0 {
		return nil, model.ErrAlertImpactMissingTag{
			TagGroups: []model.TagGroup{[]string{"src_pod", "src_namespace"}},
			Event:     event,
		}
	}

	podKey := podKey{
		Namespace: namespace,
		PodName:   podName,
	}
	if endpoints, find := endpointsMap.PodSet[podKey]; find {
		return endpoints, nil
	}

	// 通常也只会有一个Service
	services, err := repo.GetServiceListByFilter(
		startTime, endTime,
		prometheus.NamespacePQLFilter, namespace,
		prometheus.PodPQLFilter, podName,
	)
	if err != nil {
		return nil, err
	}
	var endpoints []clickhouse.AlertService
	// 通常只有一个service
	for _, service := range services {
		// 不关系ContentKey
		endpoints = append(endpoints, clickhouse.AlertService{
			ServiceName: service,
		})
	}

	if endpointsMap != nil {
		endpointsMap.PodSet[podKey] = endpoints
		for _, endpoint := range endpoints {
			endpointsMap.EndpointSet[endpoint] = struct{}{}
		}
	}

	if len(endpoints) == 0 {
		return nil, model.ErrAlertImpactNoMatchedService{
			TagGroup:  []string{"src_pod", "src_namespace"},
			TagValues: []string{podName, namespace},
		}
	}

	return endpoints, nil
}

func tryGetAlertServiceByNetSrcVM(repo prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, startTime time.Time, endTime time.Time) ([]clickhouse.AlertService, error) {
	nodeName := event.GetNetSrcNodeTag()
	pid := event.GetNetSrcPidTag()
	if len(nodeName) == 0 || len(pid) == 0 {
		return nil, model.ErrAlertImpactMissingTag{
			TagGroups: []model.TagGroup{[]string{"node", "pid"}},
			Event:     event,
		}
	}

	processKey := netSrcProcessKey{
		Node: nodeName,
		Pid:  pid,
	}
	if endpointsMap != nil {
		if endpoints, find := endpointsMap.NetSrcSet[processKey]; find {
			return endpoints, nil
		}
	}

	services, err := repo.GetServiceListByFilter(
		startTime, endTime,
		prometheus.NodePQLFilter, nodeName,
		prometheus.PidPQLFilter, pid,
	)

	if err != nil {
		return nil, err
	}

	var endpoints []clickhouse.AlertService
	for _, service := range services {
		endpoints = append(endpoints, clickhouse.AlertService{
			ServiceName: service,
		})
	}

	if endpointsMap != nil {
		endpointsMap.NetSrcSet[processKey] = endpoints

		for _, endpoint := range endpoints {
			endpointsMap.EndpointSet[endpoint] = struct{}{}
		}
	}

	if len(endpoints) == 0 {
		return nil, model.ErrAlertImpactNoMatchedService{
			TagGroup:  []string{"node_name", "pid"},
			TagValues: []string{nodeName, pid},
		}
	}

	return endpoints, nil
}

func tryGetAlertServiceByInfraNode(repo prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, startTime time.Time, endTime time.Time) ([]clickhouse.AlertService, error) {
	nodeName := event.GetInfraNodeTag()
	if len(nodeName) == 0 {
		return nil, model.ErrAlertImpactMissingTag{
			TagGroups: []model.TagGroup{[]string{"instance_name"}},
			Event:     event,
		}
	}

	if endpointsMap != nil {
		if endpoints, find := endpointsMap.InferNodeSet[nodeName]; find {
			return endpoints, nil
		}
	}

	services, err := repo.GetServiceListByFilter(
		startTime, endTime,
		prometheus.NodePQLFilter, nodeName,
	)

	if err != nil {
		return nil, err
	}

	var endpoints []clickhouse.AlertService
	for _, service := range services {
		endpoints = append(endpoints, clickhouse.AlertService{
			ServiceName: service,
		})
	}

	if endpointsMap != nil {
		endpointsMap.InferNodeSet[nodeName] = endpoints
		for _, endpoint := range endpoints {
			endpointsMap.EndpointSet[endpoint] = struct{}{}
		}
	}

	if len(endpoints) == 0 {
		return nil, model.ErrAlertImpactNoMatchedService{
			TagGroup:  []string{"node"},
			TagValues: []string{nodeName},
		}
	}

	return endpoints, nil
}

func tryGetAlertServiceByContainer(repo prometheus.Repo, endpointsMap *EndpointsMap, event *model.AlertEvent, startTime time.Time, endTime time.Time) ([]clickhouse.AlertService, error) {
	podName := event.GetK8sPodTag()
	namespace := event.GetK8sNamespaceTag()
	if len(podName) == 0 || len(namespace) == 0 {
		return nil, model.ErrAlertImpactMissingTag{
			TagGroups: []model.TagGroup{[]string{"pod", "namespace"}},
			Event:     event,
		}
	}

	podKey := podKey{
		Namespace: namespace,
		PodName:   podName,
	}
	if endpoints, find := endpointsMap.PodSet[podKey]; find {
		return endpoints, nil
	}

	// 通常也只会有一个Service
	services, err := repo.GetServiceListByFilter(
		startTime, endTime,
		prometheus.NamespacePQLFilter, namespace,
		prometheus.PodPQLFilter, podName,
	)
	if err != nil {
		return nil, err
	}
	var endpoints []clickhouse.AlertService
	// 通常只有一个service
	for _, service := range services {
		// 不关系ContentKey
		endpoints = append(endpoints, clickhouse.AlertService{
			ServiceName: service,
		})
	}

	if endpointsMap != nil {
		endpointsMap.PodSet[podKey] = endpoints
		for _, endpoint := range endpoints {
			endpointsMap.EndpointSet[endpoint] = struct{}{}
		}
	}

	if len(endpoints) == 0 {
		return nil, model.ErrAlertImpactNoMatchedService{
			TagGroup:  []string{"pod", "namespace"},
			TagValues: []string{podName, namespace},
		}
	}

	return endpoints, nil
}
