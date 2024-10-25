package alertanalyze

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/request"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	ck "github.com/CloudDetail/apo/backend/pkg/repository/clickhouse"
)

const (
	alertEventPrefix = "alert-"
	errorEvent       = "error"
	mutationEvent    = "mutation"
)

// SearchAnormalDeltaByEntry 基于入口查询异常事件
func (s *service) SearchAnormalDeltaByEntry(req *request.GetDescendantAnormalDeltaEventRequest) (*response.GetDescendantDeltaAnormalEventResponse, error) {
	startTime := time.UnixMicro(req.StartTime)
	endTime := time.UnixMicro(req.EndTime)

	descendants, err := s.chRepo.ListDescendantNodes(&request.GetDescendantMetricsRequest{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Service:   req.Service,
		Endpoint:  req.Endpoint,
	})
	if err != nil {
		return nil, err
	}

	var selectedEvents []string
	if len(req.AnormalTypes) > 0 {
		selectedEvents = strings.Split(req.AnormalTypes, ",")
	}

	// 便于后续查询受告警影响的服务
	var instances []*model.ServiceInstance
	var endpoints []model.EndpointKey
	instanceMap := newInstanceMap()
	// TODO 优化,减少查询次数

	for _, descendant := range descendants {
		// 获取每个endpoint下的所有实例
		instanceList, err := s.promRepo.GetInstanceList(req.StartTime, req.EndTime, descendant.Service, descendant.Endpoint)
		if err != nil {
			continue
		}

		// 构建好子孙节点的Node/Service -> descendant 映射
		endpoint := model.EndpointKey{
			ServiceName: descendant.Service,
			ContentKey:  descendant.Endpoint,
		}
		instancesForDescendant := instanceList.GetInstances()
		instanceMap.AddInstances(endpoint, instancesForDescendant)

		instances = append(instances, instancesForDescendant...)
		endpoints = append(endpoints, endpoint)
	}

	// 返回结果列表
	var anormalEventList []model.AnormalEvent

	// 获取匹配的error
	if len(selectedEvents) == 0 || contains(selectedEvents, errorEvent) {
		propagations, err := s.chRepo.ListErrorByEntryService(req.StartTime, req.EndTime, req.Service, req.Endpoint, endpoints)
		if err == nil {
			errorEvents := s.parseErrorEvent(propagations, instanceMap, req.Step)
			// 存放error事件
			anormalEventList = append(anormalEventList, errorEvents...)
		} else {
			return nil, err
		}
	}

	if len(selectedEvents) == 0 || hasPrefix(selectedEvents, alertEventPrefix) {
		// 获取匹配的alertEvents
		alertEvents, err := s.chRepo.GetAlertEventsWithKeyByInstanceAndEndpoints(
			startTime, endTime,
			// 同时获取Firing和Resolved故障
			request.AlertFilter{},
			instances, endpoints,
		)

		if err == nil {
			anormalAlerts := s.parseAlertEvents(alertEvents, instanceMap, selectedEvents, req.StartTime)
			// 存放告警事件
			anormalEventList = append(anormalEventList, anormalAlerts...)
		} else {
			return nil, err
		}
	}

	// if len(req.MutataionCheckPQL) > 0 && contains(selectedEvents, mutationEvent) {
	// 	// 用户自定义指标突变
	// 	anormalMutations, err := s.doMutationCheck(req, startTime, endTime, instanceMap)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	anormalEventList = append(anormalEventList, anormalMutations...)
	// }

	// sort.SliceStable(anormalEventList, func(i, j int) bool {
	// 	return anormalEventList[i].Timestamp < anormalEventList[j].Timestamp
	// })

	// 按Step分组并生成Chart, 统计每个Step时间段内未解决的异常数量
	anormalCount := response.TempChartObject{
		ChartData: map[int64]float64{},
	}
	// 初始化
	for i := req.StartTime; i < req.EndTime; i += req.Step {
		anormalCount.ChartData[i] = 0
	}

	originAnormalCounts := map[model.EndpointKey]map[model.AnormalType]int64{}
	finalAnormalCounts := map[model.EndpointKey]map[model.AnormalType]int64{}

	originAnormalEvents := []response.DescendantAnormalEventRecord{}
	deltaAnormalEvents := []response.DescendantAnormalEventRecord{}
	finalAnrmalEvents := []response.DescendantAnormalEventRecord{}

	// anormalEventTimeGroup := make(map[int64][]model.AnormalEvent)
	for _, event := range anormalEventList {
		// status Before Firing
		isFiringBefore := model.StatusResolved
		lastEventTSBefore := int64(-1)
		lastEventTSAfter := int64(-1)
		isFiringAfter := model.StatusResolved
		deltaEventTS := []model.AnormalUpdateTS{}

		// 填充Chart
		startFiring := int64(-1)
		for updateIdx, updateTS := range event.UpdateTSs {
			if updateIdx == len(event.UpdateTSs)-1 && updateTS.AnormalStatus == model.StatusFiring {
				if startFiring == -1 {
					startFiring = updateTS.Timestamp
				}
				for ts, count := range anormalCount.ChartData {
					if ts+req.Step >= startFiring {
						anormalCount.ChartData[ts] = count + 1
					}
				}
			} else if startFiring == -1 && updateTS.AnormalStatus == model.StatusFiring {
				// 新的一次告警
				startFiring = updateTS.Timestamp
			} else if startFiring != -1 && updateTS.AnormalStatus == model.StatusResolved {
				// 结算之前的告警
				for ts, count := range anormalCount.ChartData {
					if ts+req.Step >= startFiring && ts+req.Step < updateTS.Timestamp {
						anormalCount.ChartData[ts] = count + 1
					}
				}
			}

			// 统计用户时间片前的状态
			if updateTS.Timestamp < req.DeltaStartTime {
				isFiringBefore = updateTS.AnormalStatus
				lastEventTSBefore = updateTS.Timestamp
			}
			if updateTS.Timestamp < req.DeltaEndTime {
				isFiringAfter = updateTS.AnormalStatus
				lastEventTSAfter = updateTS.Timestamp
				if updateTS.Timestamp > req.DeltaStartTime {
					deltaEventTS = append(deltaEventTS, updateTS)
				}
			}
		}

		if isFiringBefore == model.StatusFiring {
			for _, impactEndpoints := range event.ImpactEndpoints {
				alertCounts, find := originAnormalCounts[impactEndpoints.EndpointKey]
				if !find {
					alertCounts = map[model.AnormalType]int64{}
					originAnormalCounts[impactEndpoints.EndpointKey] = alertCounts
				}
				alertCounts[event.AnormalType]++
			}

			// 记录已经发生的告警事件和当时的状态
			for _, impactEndpoint := range event.ImpactEndpoints {
				originAnormalEvents = append(originAnormalEvents, response.DescendantAnormalEventRecord{
					EndpointKey:   impactEndpoint.EndpointKey,
					AlertKey:      impactEndpoint.GetEventKey(),
					AnormalType:   event.AnormalType,
					Timestamp:     lastEventTSBefore,
					AnormalObject: impactEndpoint.AlertObject,
					AnormalReason: impactEndpoint.AlertReason,
					AnormalMsg:    impactEndpoint.AlertMessage[lastEventTSBefore],
					AnormalStatus: "updatedFiring",
				})
			}
		}

		if isFiringAfter == model.StatusFiring {
			for _, impactEndpoints := range event.ImpactEndpoints {
				alertCounts, find := finalAnormalCounts[impactEndpoints.EndpointKey]
				if !find {
					alertCounts = map[model.AnormalType]int64{}
					finalAnormalCounts[impactEndpoints.EndpointKey] = alertCounts
				}
				alertCounts[event.AnormalType]++
			}

			// 记录已经发生的告警事件和当时的状态
			for _, impactEndpoint := range event.ImpactEndpoints {
				finalAnrmalEvents = append(finalAnrmalEvents, response.DescendantAnormalEventRecord{
					EndpointKey:   impactEndpoint.EndpointKey,
					AlertKey:      impactEndpoint.GetEventKey(),
					AnormalType:   event.AnormalType,
					Timestamp:     lastEventTSAfter,
					AnormalObject: impactEndpoint.AlertObject,
					AnormalReason: impactEndpoint.AlertReason,
					AnormalMsg:    impactEndpoint.AlertMessage[lastEventTSAfter],
					AnormalStatus: "updatedFiring",
				})
			}
		}

		if len(deltaEventTS) > 0 {
			// 记录发生变化的事件 (create / update / resolved)
			for _, impactEndpoint := range event.ImpactEndpoints {
				var hasStarted bool
				for _, ts := range deltaEventTS {
					if event.AnormalType == model.AnormalTypeError && ts.AnormalStatus == model.StatusResolved {
						continue // 不记录Error事件的Resolved
					}
					var anormalStatus string
					if isFiringBefore == model.StatusFiring && ts.AnormalStatus == model.StatusFiring ||
						hasStarted && isFiringBefore == model.StatusFiring && ts.AnormalStatus == model.StatusFiring {
						anormalStatus = "updatedFiring"
					} else if isFiringBefore == model.StatusResolved && ts.AnormalStatus == model.StatusFiring {
						anormalStatus = "startFiring"
						hasStarted = true
					} else if isFiringBefore == model.StatusFiring && ts.AnormalStatus == model.StatusResolved ||
						hasStarted && ts.AnormalStatus == model.StatusResolved {
						anormalStatus = "resolved"
					}

					deltaAnormalEvents = append(deltaAnormalEvents, response.DescendantAnormalEventRecord{
						EndpointKey:   impactEndpoint.EndpointKey,
						AlertKey:      impactEndpoint.GetEventKey(),
						AnormalType:   event.AnormalType,
						Timestamp:     ts.Timestamp,
						AnormalObject: impactEndpoint.AlertObject,
						AnormalReason: impactEndpoint.AlertReason,
						AnormalMsg:    impactEndpoint.AlertMessage[ts.Timestamp],
						AnormalStatus: anormalStatus,
					})
				}
			}
		}
	}

	var originAnormalCountsList = []response.DescendantAnormalCounts{}
	for endpoint, anormalCounts := range originAnormalCounts {
		originAnormalCountsList = append(originAnormalCountsList, response.DescendantAnormalCounts{
			EndpointKey:      endpoint,
			AnormalCountsMap: anormalCounts,
		})
	}

	var finalAnormalCountsList = []response.DescendantAnormalCounts{}
	for endpoint, anormalCounts := range finalAnormalCounts {
		finalAnormalCountsList = append(finalAnormalCountsList, response.DescendantAnormalCounts{
			EndpointKey:      endpoint,
			AnormalCountsMap: anormalCounts,
		})
	}

	return &response.GetDescendantDeltaAnormalEventResponse{
		AnormalCount:        anormalCount,
		OriginAnormalCounts: originAnormalCountsList,
		FinalAnormalCounts:  finalAnormalCountsList,

		OriginAnormalEvents: originAnormalEvents,
		DeltaAnormalEvents:  deltaAnormalEvents,
		FinalAnormalEvents:  finalAnrmalEvents,
	}, nil
}

// func (s *service) doMutationCheck(req *request.GetDescendantAnormalEventRequest, startTime time.Time, endTime time.Time, instanceMap *InstanceMap) ([]model.AnormalEvent, error) {
// 	mutationCheck := &prometheus.MutationPQLCheck{
// 		PQL:        req.MutataionCheckPQL,
// 		UpperLimit: req.MutationUpperLimit,
// 		LowerLimit: req.MutationLowerLimit,
// 	}
// 	mutationSeries, err := s.promRepo.ExecutedMutationCheck(
// 		mutationCheck,
// 		startTime, endTime,
// 		time.Duration(req.Step)*time.Microsecond,
// 	)
// 	if err != nil {
// 		return nil, model.ErrMutationCheckFailed{
// 			PQL:        req.MutataionCheckPQL,
// 			UpperLimit: req.MutationUpperLimit,
// 			LowerLimit: req.MutationLowerLimit,
// 			UserMsg:    "自定义指标语法异常",
// 			Err:        err,
// 		}
// 	}

// 	var anormalEventList []model.AnormalEvent
// 	for _, serie := range mutationSeries {
// 		if len(serie.Metric.SvcName) > 0 && len(serie.Metric.ContentKey) > 0 {
// 			labelsStr := fmt.Sprintf("%+v", serie.Metric)
// 			for _, point := range serie.Values {
// 				var anormalEvent model.AnormalEvent = model.AnormalEvent{
// 					Timestamp:       point.TimeStamp,
// 					AnormalType:     model.AnormalTypeMutation,
// 					ImpactEndpoints: []model.AnormalEventDetail{},
// 				}
// 				anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
// 					EndpointKey: model.EndpointKey{
// 						ServiceName: serie.Metric.SvcName,
// 						ContentKey:  serie.Metric.ContentKey,
// 					},
// 					AlertObject:  serie.Metric.SvcName,
// 					AlertReason:  "应用关联指标突变",
// 					AlertMessage: mutationCheck.GetMurationMessage(point.Value, labelsStr),
// 				})
// 				anormalEventList = append(anormalEventList, anormalEvent)
// 			}
// 		} else if len(serie.Metric.Namespace) > 0 && len(serie.Metric.POD) > 0 {
// 			instance, endpoints := instanceMap.GetEndpointsByK8sPodNS(serie.Metric.POD, serie.Metric.Namespace)
// 			if instance == nil {
// 				continue
// 			}

// 			labelsStr := fmt.Sprintf("%+v", serie.Metric)
// 			for _, point := range serie.Values {
// 				var anormalEvent model.AnormalEvent = model.AnormalEvent{
// 					Timestamp:       point.TimeStamp,
// 					AnormalType:     model.AnormalTypeMutation,
// 					ImpactEndpoints: []model.AnormalEventDetail{},
// 				}
// 				for _, endpoint := range endpoints {
// 					anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
// 						EndpointKey:  endpoint,
// 						AlertObject:  instance.PodName,
// 						AlertReason:  "应用关联指标突变",
// 						AlertMessage: mutationCheck.GetMurationMessage(point.Value, labelsStr),
// 					})
// 				}
// 				anormalEventList = append(anormalEventList, anormalEvent)
// 			}
// 		} else if len(serie.Metric.PID) > 0 {
// 			instance, endpoints := instanceMap.GetEndpointsByNodePid(serie.Metric.NodeName, serie.Metric.PID)
// 			if instance == nil {
// 				continue
// 			}

// 			labelsStr := fmt.Sprintf("%+v", serie.Metric)
// 			for _, point := range serie.Values {
// 				var anormalEvent model.AnormalEvent = model.AnormalEvent{
// 					Timestamp:       point.TimeStamp,
// 					AnormalType:     model.AnormalTypeMutation,
// 					ImpactEndpoints: []model.AnormalEventDetail{},
// 				}
// 				for _, endpoint := range endpoints {
// 					anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
// 						EndpointKey:  endpoint,
// 						AlertObject:  fmt.Sprintf("(pid:%s) at %s", serie.Metric.PID, instance.NodeName),
// 						AlertReason:  "应用关联指标突变",
// 						AlertMessage: mutationCheck.GetMurationMessage(point.Value, labelsStr),
// 					})
// 				}
// 				anormalEventList = append(anormalEventList, anormalEvent)
// 			}
// 		} else if len(serie.Metric.NodeName) > 0 {
// 			endpointsMaps := instanceMap.GetEndpointsByNode(serie.Metric.NodeName)
// 			if endpointsMaps == nil {
// 				continue
// 			}

// 			for instance, endpoints := range endpointsMaps {
// 				var instanceName string
// 				if len(instance.PodName) > 0 {
// 					instanceName = fmt.Sprintf("%s/%s", instance.Namespace, instance.PodName)
// 				} else {
// 					instanceName = fmt.Sprintf("(pid:%d)", instance.Pid)
// 				}

// 				for _, point := range serie.Values {
// 					var anormalEvent model.AnormalEvent = model.AnormalEvent{
// 						Timestamp:       point.TimeStamp,
// 						AnormalType:     model.AnormalTypeMutation,
// 						ImpactEndpoints: []model.AnormalEventDetail{},
// 					}
// 					for _, endpoint := range endpoints {
// 						anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
// 							EndpointKey:  endpoint,
// 							AlertObject:  instanceName + " at " + instance.NodeName,
// 							AlertReason:  "应用所在主机指标突变",
// 							AlertMessage: mutationCheck.GetMurationMessage(point.Value, fmt.Sprintf("%+v", serie.Metric)),
// 						})
// 					}
// 					anormalEventList = append(anormalEventList, anormalEvent)
// 				}
// 			}
// 		} else {
// 			return nil, model.ErrMutationCheckFailed{
// 				PQL:        req.MutataionCheckPQL,
// 				UpperLimit: req.MutationUpperLimit,
// 				LowerLimit: req.MutationLowerLimit,
// 				UserMsg:    "指标无法关联到具体服务,Label中需要存在(namespace,pod),(node,pid),(node)组合之一",
// 				Err:        fmt.Errorf("指标无法关联到服务实例: Metric :%s, Labels: %+v\nLabels需要包含下面的label组合之一: (namespace,pod),(node,pid),(node)", req.MutataionCheckPQL, serie.Metric),
// 			}
// 		}
// 	}
// 	return anormalEventList, nil
// }

func (*service) parseErrorEvent(propagations []ck.ErrorPropation, instanceMap *InstanceMap, step int64) []model.AnormalEvent {
	var anormalEventList []model.AnormalEvent
	for _, propagation := range propagations {
		errorEvent := model.AnormalEvent{
			UpdateTSs: []model.AnormalUpdateTS{
				{
					AnormalStatus: model.StatusFiring,
					Timestamp:     propagation.Timestamp.UnixMicro(),
				},
				{
					// 自定义一个一分钟结束的状态
					AnormalStatus: model.StatusResolved,
					Timestamp:     propagation.Timestamp.Add(time.Duration(step * 1000)).UnixMicro(),
				},
			},
			AnormalType:     model.AnormalTypeError,
			ImpactEndpoints: []model.AnormalEventDetail{},
		}
		for idx, service := range propagation.NodesService {
			if !propagation.NodesIsError[idx] {
				continue
			}
			// 检查数据存在,防止数组越界
			if len(propagation.NodesUrl) <= idx ||
				len(propagation.NodesInstance) <= idx ||
				len(propagation.NodesErrorTypes) <= idx ||
				len(propagation.NodesErrorMsgs) <= idx {
				break
			}
			endpointKey := model.EndpointKey{
				ServiceName: service,
				ContentKey:  propagation.NodesUrl[idx],
			}
			// 检查ContentKey是否在endpoints中
			if !instanceMap.IsEndpointKeyExist(endpointKey) {
				continue
			}
			errorEvent.ImpactEndpoints = append(errorEvent.ImpactEndpoints, model.AnormalEventDetail{
				EndpointKey: endpointKey,
				AlertKey:    model.GenUUID().String(),
				AlertObject: propagation.NodesInstance[idx],
				AlertReason: strings.Join(propagation.NodesErrorTypes[idx], ";"),
				AlertMessage: map[int64]string{
					propagation.Timestamp.UnixMicro(): strings.Join(propagation.NodesErrorMsgs[idx], ";"),
				},
			})
		}
		anormalEventList = append(anormalEventList, errorEvent)
	}
	return anormalEventList
}

func (*service) parseAlertEvents(alertEvents []ck.AlertEventWithKey, instanceMap *InstanceMap, selectedEvents []string, searchStartTime int64) []model.AnormalEvent {
	var anormalEventList []model.AnormalEvent

	for i := 0; i < len(alertEvents); i++ {
		alertEvent := alertEvents[i]
		if len(selectedEvents) > 0 && !contains(selectedEvents, alertEventPrefix+alertEvent.Group) {
			// 跳过未选择的事件
			continue
		}
		if alertEvent.Status == model.StatusResolved {
			// 跳过第一个状态就是Resolved的事件
			continue
		}

		// 初始化每个告警的首个事件
		var updateTSs []model.AnormalUpdateTS
		var alertMessage = map[int64]string{}
		if alertEvent.CreateTime.UnixMicro() < searchStartTime {
			updateTSs = append(updateTSs, model.AnormalUpdateTS{
				Timestamp:     alertEvent.CreateTime.UnixMicro(),
				AnormalStatus: model.StatusFiring,
			})
			alertMessage = map[int64]string{
				alertEvent.CreateTime.UnixMicro(): "在查询时间开始前已告警,初次告警在" + alertEvent.CreateTime.Format("2006-01-02 15:04:05"),
			}
		}
		updateTSs = append(updateTSs, model.AnormalUpdateTS{
			Timestamp:     alertEvent.ReceivedTime.UnixMicro(),
			AnormalStatus: alertEvent.Status,
		})
		alertMessage[alertEvent.ReceivedTime.UnixMicro()] = alertEvent.Detail
		var anormalEvent model.AnormalEvent = model.AnormalEvent{
			UpdateTSs:       updateTSs,
			ImpactEndpoints: []model.AnormalEventDetail{},
		}

		// 完善告警信息
		switch ck.AlertGroup(alertEvent.Group) {
		case ck.APP_GROUP:
			anormalEvent.AnormalType = model.AnormalTypeAlertApp
			anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
				EndpointKey: model.EndpointKey{
					ServiceName: alertEvent.GetServiceNameTag(),
					ContentKey:  alertEvent.GetContentKeyTag(),
				},
				AlertKey:     alertEvent.AlertKey,
				AlertObject:  alertEvent.GetTargetObj(),
				AlertReason:  alertEvent.Name,
				AlertMessage: alertMessage,
			})
		case ck.CONTAINER_GROUP:
			anormalEvent.AnormalType = model.AnormalTypeAlertContainer
			instance, endpoints := instanceMap.GetEndpointsByK8sPodNS(alertEvent.GetK8sPodTag(), alertEvent.GetK8sNamespaceTag())
			if instance == nil {
				continue
			}
			for _, endpoint := range endpoints {
				anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
					EndpointKey:  endpoint,
					AlertKey:     alertEvent.AlertKey,
					AlertObject:  alertEvent.GetTargetObj(),
					AlertReason:  alertEvent.Name,
					AlertMessage: alertMessage,
				})
			}
		case ck.NETWORK_GROUP:
			anormalEvent.AnormalType = model.AnormalTypeAlertNet
			var endpoints []model.EndpointKey
			var instance *model.ServiceInstance
			pod := alertEvent.GetK8sPodTag()
			if len(pod) > 0 {
				instance, endpoints = instanceMap.GetEndpointsByK8sPodNS(alertEvent.GetK8sPodTag(), alertEvent.GetK8sNamespaceTag())
				if instance == nil {
					continue
				}
			} else {
				instance, endpoints = instanceMap.GetEndpointsByNodePid(alertEvent.GetNetSrcNodeTag(), alertEvent.GetNetSrcPidTag())
				if instance == nil {
					continue
				}
			}
			for _, endpoint := range endpoints {
				anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
					EndpointKey:  endpoint,
					AlertKey:     alertEvent.AlertKey,
					AlertObject:  alertEvent.GetTargetObj(),
					AlertReason:  alertEvent.Name,
					AlertMessage: alertMessage,
				})
			}
		case ck.INFRA_GROUP:
			anormalEvent.AnormalType = model.AnormalTypeAlertInfra
			endpointsMaps := instanceMap.GetEndpointsByNode(alertEvent.GetInfraNodeTag())
			for instance, endpoints := range endpointsMaps {
				var instanceName string
				if len(instance.PodName) > 0 {
					instanceName = fmt.Sprintf("%s/%s", instance.Namespace, instance.PodName)
				} else {
					instanceName = fmt.Sprintf("(pid:%d)", instance.Pid)
				}
				for _, endpoint := range endpoints {
					anormalEvent.ImpactEndpoints = append(anormalEvent.ImpactEndpoints, model.AnormalEventDetail{
						EndpointKey:  endpoint,
						AlertKey:     alertEvent.AlertKey,
						AlertObject:  instanceName + " at " + alertEvent.GetTargetObj(),
						AlertReason:  alertEvent.Name,
						AlertMessage: alertMessage,
					})
				}
			}
		}

		var j = i + 1
		var lastStatus model.Status
		for j < len(alertEvents) {
			nextEvent := alertEvents[j]
			if nextEvent.AlertKey != alertEvent.AlertKey {
				break
			}
			// 跳过重复的resolved事件
			if nextEvent.Status != model.StatusResolved || lastStatus != model.StatusResolved {
				anormalEvent.UpdateTSs = append(anormalEvent.UpdateTSs, model.AnormalUpdateTS{
					Timestamp:     nextEvent.ReceivedTime.UnixMicro(),
					AnormalStatus: nextEvent.Status,
				})
				for _, endpoint := range anormalEvent.ImpactEndpoints {
					endpoint.AlertMessage[nextEvent.ReceivedTime.UnixMicro()] = nextEvent.Detail
				}
				lastStatus = nextEvent.Status
			}
			j++
			// 后续处理时跳过已经完成处理的事件
			i = j + 1
		}
		anormalEventList = append(anormalEventList, anormalEvent)
	}
	return anormalEventList
}

type InstanceMap struct {
	Pod2InstanceMap     map[K8sPodNSKey]model.ServiceInstance
	NodePid2InstanceMap map[NodePidKey]model.ServiceInstance
	Node2InstancesMap   map[string]map[model.ServiceInstance]struct{}

	InstanceMap map[model.ServiceInstance]map[model.EndpointKey]struct{}

	EndpointMap map[model.EndpointKey]struct{}
}

type NodePidKey struct {
	Node string
	Pid  int
}

type K8sPodNSKey struct {
	Namespace string
	Pod       string
}

func newInstanceMap() *InstanceMap {
	return &InstanceMap{
		Pod2InstanceMap:     map[K8sPodNSKey]model.ServiceInstance{},
		NodePid2InstanceMap: map[NodePidKey]model.ServiceInstance{},
		Node2InstancesMap:   map[string]map[model.ServiceInstance]struct{}{},
		InstanceMap:         map[model.ServiceInstance]map[model.EndpointKey]struct{}{},
		EndpointMap:         map[model.EndpointKey]struct{}{},
	}
}

func (m *InstanceMap) AddInstances(endpointKey model.EndpointKey, instances []*model.ServiceInstance) {
	m.EndpointMap[endpointKey] = struct{}{}

	for _, instance := range instances {
		endpointKeys, find := m.InstanceMap[*instance]
		if !find {
			endpointKeys = make(map[model.EndpointKey]struct{})
		}
		endpointKeys[endpointKey] = struct{}{}
		m.InstanceMap[*instance] = endpointKeys

		if len(instance.PodName) > 0 {
			m.Pod2InstanceMap[K8sPodNSKey{instance.Namespace, instance.PodName}] = *instance
		}

		if instance.Pid > 0 {
			m.NodePid2InstanceMap[NodePidKey{instance.NodeName, int(instance.Pid)}] = *instance
		}

		if len(instance.NodeName) > 0 {
			instancesOnNode, find := m.Node2InstancesMap[instance.NodeName]
			if !find {
				instancesOnNode = make(map[model.ServiceInstance]struct{})
			}
			instancesOnNode[*instance] = struct{}{}
			m.Node2InstancesMap[instance.NodeName] = instancesOnNode
		}
	}
}

func (m *InstanceMap) GetEndpointsByK8sPodNS(pod, namespace string) (*model.ServiceInstance, []model.EndpointKey) {
	instance, find := m.Pod2InstanceMap[K8sPodNSKey{namespace, pod}]
	if !find {
		return nil, nil
	}

	endpointsMap, find := m.InstanceMap[instance]
	if !find {
		return nil, nil
	}
	var endpoints []model.EndpointKey
	for endpoint := range endpointsMap {
		endpoints = append(endpoints, endpoint)
	}
	return &instance, endpoints
}

func (m *InstanceMap) GetEndpointsByNodePid(node string, pid string) (*model.ServiceInstance, []model.EndpointKey) {
	if len(pid) == 0 {

	}
	pidInt, err := strconv.Atoi(pid)
	if err != nil {
		return nil, nil
	}

	instance, find := m.NodePid2InstanceMap[NodePidKey{node, pidInt}]
	if !find {
		return nil, nil
	}

	endpointsMap, find := m.InstanceMap[instance]
	if !find {
		return nil, nil
	}
	var endpoints []model.EndpointKey
	for endpoint := range endpointsMap {
		endpoints = append(endpoints, endpoint)
	}
	return &instance, endpoints
}

func (m *InstanceMap) GetEndpointsByNode(node string) map[model.ServiceInstance][]model.EndpointKey {
	instances, find := m.Node2InstancesMap[node]
	if !find || len(instances) == 0 {
		return nil
	}

	var res = make(map[model.ServiceInstance][]model.EndpointKey)
	for instance := range instances {
		endpointsMap, find := m.InstanceMap[instance]
		if !find {
			continue
		}
		var endpoints []model.EndpointKey
		for endpoint := range endpointsMap {
			endpoints = append(endpoints, endpoint)
		}

		res[instance] = endpoints
	}

	return res
}

func (m *InstanceMap) IsEndpointKeyExist(endpointKey model.EndpointKey) bool {
	_, find := m.EndpointMap[endpointKey]
	return find
}

func contains(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

func hasPrefix(arr []string, str string) bool {
	for _, v := range arr {
		if strings.HasPrefix(v, str) {
			return true
		}
	}
	return false
}
