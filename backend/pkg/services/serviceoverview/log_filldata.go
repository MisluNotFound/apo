package serviceoverview

import (
	"math"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/CloudDetail/apo/backend/pkg/repository/prometheus"
)

func (s *service) AvgLogByPod(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogPromql(duration, prometheus.AvgLog, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		podName := result.Metric.PodName
		if podName == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == podName {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].AvgLog = &value
				}
				break
			}
		}
	}
	return Instances, err
}

func (s *service) LogDODByPod(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogPromql(duration, prometheus.LogDOD, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		podName := result.Metric.PodName
		if podName == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == podName {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogDayOverDay = &value
				} else {
					var value float64
					value = RES_MAX_VALUE
					pointer := &value
					(*Instances)[i].LogDayOverDay = pointer
				}
				break
			}
		}
	}
	return Instances, err
}
func (s *service) LogWOWByPod(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogPromql(duration, prometheus.LogWOW, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		podName := result.Metric.PodName
		if podName == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == podName {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogWeekOverWeek = &value
				} else {
					var value float64
					value = RES_MAX_VALUE
					pointer := &value
					(*Instances)[i].LogWeekOverWeek = pointer
				}
				break
			}
		}
	}
	return Instances, err
}

// 查询曲线图

func (s *service) LogRangeDataByPod(Instances *[]Instance, pods []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*[]Instance, error) {
	if Instances == nil {
		return nil, errors.New("instances is nil")
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogPromql(stepToStr, prometheus.AvgLog, pods)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		podName := result.Metric.PodName
		if podName == "" {
			continue
		}
		for i, instance := range *Instances {
			if instance.ConvertName == podName {
				(*Instances)[i].LogData = result.Values
				break
			}
		}
	}

	return Instances, err
}

func (s *service) AvgLogByContainerId(Instances *[]Instance, containerIds []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByContainerIdPromql(duration, prometheus.AvgLog, containerIds)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		containerId := result.Metric.ContainerID
		if containerId == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == containerId {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].AvgLog = &value
				}
				break
			}
		}
	}
	return Instances, err
}

func (s *service) LogDODByContainerId(Instances *[]Instance, containerIds []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByContainerIdPromql(duration, prometheus.LogDOD, containerIds)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		contianerId := result.Metric.ContainerID
		if contianerId == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == contianerId {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogDayOverDay = &value
				}
				break
			}
		}
	}
	return Instances, err
}
func (s *service) LogWOWByContainerId(Instances *[]Instance, containerIds []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByContainerIdPromql(duration, prometheus.LogWOW, containerIds)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		containerId := result.Metric.ContainerID
		if containerId == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == containerId {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogWeekOverWeek = &value
				}
				break
			}
		}
	}
	return Instances, err
}

// 查询曲线图

func (s *service) LogRangeDataByContainerId(Instances *[]Instance, containerIds []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*[]Instance, error) {
	if Instances == nil {
		return nil, errors.New("instances is nil")
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogByContainerIdPromql(stepToStr, prometheus.AvgLog, containerIds)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		containerId := result.Metric.ContainerID
		if containerId == "" {
			continue
		}
		for i, instance := range *Instances {
			if instance.ConvertName == containerId {
				(*Instances)[i].LogData = result.Values
				break
			}
		}
	}

	return Instances, err
}

func (s *service) AvgLogByPid(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByPidPromql(duration, prometheus.AvgLog, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		pid := result.Metric.PID
		if pid == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == pid {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].AvgLog = &value
				}
				break
			}
		}
	}
	return Instances, err
}

func (s *service) LogDODByPid(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByPidPromql(duration, prometheus.LogDOD, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		pid := result.Metric.PID
		if pid == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == pid {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogDayOverDay = &value
				}
				break
			}
		}
	}
	return Instances, err
}
func (s *service) LogWOWByPid(Instances *[]Instance, pods []string, endTime time.Time, duration string) (*[]Instance, error) {
	var LogRateRes []prometheus.MetricResult
	queryLog := prometheus.QueryLogByPidPromql(duration, prometheus.LogWOW, pods)
	LogRateRes, err := s.promRepo.QueryData(endTime, queryLog)
	for _, result := range LogRateRes {
		pid := result.Metric.PID
		if pid == "" {
			continue
		}
		value := result.Values[0].Value
		for i, Instance := range *Instances {
			if Instance.ConvertName == pid {
				if !math.IsInf(value, 0) { //为无穷大时则不赋值
					(*Instances)[i].LogWeekOverWeek = &value
				}
				break
			}
		}
	}
	return Instances, err
}

// 查询曲线图

func (s *service) LogRangeDataByPid(Instances *[]Instance, pods []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*[]Instance, error) {
	if Instances == nil {
		return nil, errors.New("instances is nil")
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogByPidPromql(stepToStr, prometheus.AvgLog, pods)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		pid := result.Metric.PID
		if pid == "" {
			continue
		}
		for i, instance := range *Instances {
			if instance.ConvertName == pid {
				(*Instances)[i].LogData = result.Values
				break
			}
		}
	}

	return Instances, err
}

func (s *service) ServiceLogRangeDataByPod(Service *ServiceDetail, pods []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*ServiceDetail, error) {
	if Service == nil {
		return nil, errors.New("service is nil")
	}
	if pods == nil {
		return Service, nil
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogPromql(stepToStr, prometheus.AvgLog, pods)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		Service.LogData = result.Values
		break
	}

	return Service, err
}

func (s *service) ServiceLogRangeDataByContainerId(Service *ServiceDetail, containerIds []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*ServiceDetail, error) {
	if Service == nil {
		return nil, errors.New("service is nil")
	}
	if containerIds == nil {
		return Service, nil
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogPromql(stepToStr, prometheus.AvgLog, containerIds)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		Service.LogData = result.Values
		break
	}

	return Service, err
}

func (s *service) ServiceLogRangeDataByPid(Service *ServiceDetail, pids []string, startTime time.Time, endTime time.Time, duration string, step time.Duration) (*ServiceDetail, error) {
	if Service == nil {
		return nil, errors.New("service is nil")
	}
	if pids == nil {
		return Service, nil
	}
	var stepToStr string
	if step >= time.Hour {
		stepToStr = strconv.FormatInt(int64(step/time.Hour), 10) + "h"
	} else if step >= time.Minute {
		stepToStr = strconv.FormatInt(int64(step/time.Minute), 10) + "m"
	} else {
		stepToStr = strconv.FormatInt(int64(step/time.Second), 10) + "s"
	}
	//LogDataRes, err = s.promRepo.QueryRangePrometheusErrorLast30min(searchTime)
	LogDataQuery := prometheus.QueryLogPromql(stepToStr, prometheus.AvgLog, pids)
	LogDataRes, err := s.promRepo.QueryRangeData(startTime, endTime, LogDataQuery, step)
	for _, result := range LogDataRes {
		Service.LogData = result.Values
		break
	}

	return Service, err
}
