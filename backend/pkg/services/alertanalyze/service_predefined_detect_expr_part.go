package alertanalyze

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/CloudDetail/apo/backend/pkg/model"
	"github.com/CloudDetail/apo/backend/pkg/model/response"
	"github.com/CloudDetail/apo/backend/pkg/repository/kubernetes"
)

// 记录预定义的部分指标表达式
var (
	// 只有 percentileLantency 带Histogram Modifer
	percentileLantency = model.PredefinedMetric{
		Name:   "请求延时百分位数",
		Metric: "histogram_quantile(%Quantile%, sum by (%BucketRange%,svc_name, content_key) (increase(kindling_span_trace_duration_nanoseconds_bucket[%RangeVector%] %OFFSET%)))",
	}
	avgLantency = model.PredefinedMetric{
		Name:   "平均请求延时(s)",
		Metric: "sum by (svc_name, content_key) (increase(kindling_span_trace_duration_nanoseconds_sum[%RangeVector%] %OFFSET% ))/ sum by (svc_name, content_key) (increase(kindling_span_trace_duration_nanoseconds_count[%RangeVector%] %OFFSET%))/1000000",
	}
	errorRate = model.PredefinedMetric{
		Name:   "请求错误率(%)",
		Metric: "sum by (svc_name, content_key) (increase(kindling_span_trace_duration_nanoseconds_count{is_error=\"true\"}[%RangeVector%] %OFFSET% ))/ sum by (svc_name, content_key) (increase(kindling_span_trace_duration_nanoseconds_count[%RangeVector%] %OFFSET%))",
	}

	gaugeGroup = map[string]map[string][]model.PredefinedMetric{
		kubernetes.MutationContainerLabelVal: {
			"内存使用量": {
				{
					Name:   "容器 memory cache 量 (bytes)",
					Metric: "container_memory_cache{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 使用量(mapped_file) (bytes)",
					Metric: "container_memory_mapped_file{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 使用量(RSS) (bytes)",
					Metric: "container_memory_rss{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 使用量(Swap) (bytes)",
					Metric: "container_memory_swap{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 使用量(Usage) (bytes)",
					Metric: "container_memory_usage_bytes{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 使用量(Working Set) (bytes)",
					Metric: "container_memory_working_set_bytes{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
				{
					Name:   "容器 memory 限制量 (bytes)",
					Metric: "container_spec_memory_limit_bytes{image!=\"\", image!~\".*pause.*\"} %OFFSET%",
				},
			},
			"磁盘使用量": {
				{
					Name:   "容器 filesystem 使用量 (bytes)",
					Metric: "container_fs_usage_bytes %OFFSET%",
				},
				{
					Name:   "容器 filesystem 总量 (bytes)",
					Metric: "container_fs_limit_bytes %OFFSET%",
				},
			},
			"inodes使用量": {
				{
					Name:   "容器 inode free 量 (count)",
					Metric: "container_fs_inodes_free %OFFSET%",
				},
				{
					Name:   "容器 inode total 量 (count)",
					Metric: "container_fs_inodes_total %OFFSET%",
				},
			},
		},
	}

	// 瞬时指标
	gaugeMetric = map[string][]model.PredefinedMetric{
		kubernetes.MutationInfraLabelVal: {
			{
				Name:   "磁盘使用率 (%)",
				Metric: "((node_filesystem_avail_bytes %OFFSET% * 100) / node_filesystem_size_bytes %OFFSET% and ON (instance_name, device, mountpoint) node_filesystem_readonly %OFFSET% == 0) * on(instance_name) group_left (nodename) node_uname_info {nodename=~\".+\"} %OFFSET%",
			},
		},
		kubernetes.MutationNetLabelVal: {
			{
				Name:   "网络RTT延时 (ms)",
				Metric: "kindling_network_rtt{} %OFFSET% * 1000",
			},
		},
		kubernetes.MutationContainerLabelVal: {
			{
				Name:   "容器上次运行时间 (s)",
				Metric: "container_last_seen %OFFSET% ",
			},
			{
				Name:   "容器持久卷使用率 (%)",
				Metric: "(sum(container_fs_inodes_free{name!=\"\"} %OFFSET%) BY (instance_name) / sum(container_fs_inodes_total %OFFSET%) BY (instance_name)) * 100",
			},
			{
				Name:   "容器 filesystem 使用率 (%)",
				Metric: "container_fs_usage_bytes %OFFSET% / container_fs_limit_bytes %OFFSET% * 100",
			},
			{
				Name:   "容器 filesystem 当前 IO 次数 (count)",
				Metric: "container_fs_io_current %OFFSET%",
			},
			{
				Name:   "容器 inode 使用率 (%)",
				Metric: "100 - container_fs_inodes_free %OFFSET% / container_fs_inodes_total %OFFSET% * 100",
			},
			{
				Name:   "容器 memory 使用率(Usage) (%)",
				Metric: "100 * container_memory_usage_bytes %OFFSET% /container_spec_memory_limit_bytes %OFFSET% and container_spec_memory_limit_bytes %OFFSET% != 0",
			},
			{
				Name:   "容器 memory 使用率(Working Set) (%)",
				Metric: "100 * container_memory_working_set_bytes %OFFSET% /container_spec_memory_limit_bytes %OFFSET%  and container_spec_memory_limit_bytes %OFFSET% != 0",
			},
		},
	}

	// 累计指标
	counterMetric = map[string][]model.PredefinedMetric{
		kubernetes.MutationInfraLabelVal: {
			{
				Name:   "磁盘IO利用率 (%)",
				Metric: "(rate(node_disk_io_time_seconds_total[%RangeVector%] %OFFSET% )) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
			{
				Name:   "网络入吞吐量 (MB/s)",
				Metric: "(sum by (instance_name) (rate(node_network_receive_bytes_total[%RangeVector%] %OFFSET%)) / 1024 / 1024) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
			{
				Name:   "磁盘读速度 (MB/s)",
				Metric: "(sum by (instance_name) (rate(node_disk_read_bytes_total[%RangeVector%] %OFFSET%)) / 1024 / 1024) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
			{
				Name:   "磁盘写速度 (MB/s)",
				Metric: "(sum by (instance_name) (rate(node_disk_written_bytes_total[%RangeVector%] %OFFSET% )) / 1024 / 1024 ) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
			{
				Name:   "CPU负载 (%)",
				Metric: "(sum by (instance_name) (avg by (mode, instance_name) (rate(node_cpu_seconds_total{mode!=\"idle\"}[%RangeVector%] %OFFSET%)))) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
			{
				Name:   "CPU IO Wait (%)",
				Metric: "(avg by (instance_name) (rate(node_cpu_seconds_total{mode=\"iowait\"}[%RangeVector%] %OFFSET%)) * 100) * on(instance_name) group_left (nodename) node_uname_info{nodename=~\".+\"} %OFFSET%",
			},
		},
		kubernetes.MutationContainerLabelVal: {
			{
				Name:   "容器CPU使用率 (%)",
				Metric: "(sum(rate(container_cpu_usage_seconds_total{container!=\"\"}[%RangeVector%] %OFFSET% )) by (pod, container) / sum(container_spec_cpu_quota{container!=\"\"} %OFFSET% /container_spec_cpu_period{container!=\"\"} %OFFSET% ) by (pod, container) * 100)",
			},
			{
				Name:   "容器cpu_cfs_throttled (%)",
				Metric: "sum(increase(container_cpu_cfs_throttled_periods_total{container!=\"\"}[%RangeVector%] %OFFSET%)) by (container, pod, namespace) / sum(increase(container_cpu_cfs_periods_total[%RangeVector%] %OFFSET%)) by (container, pod, namespace)",
			},
			{
				Name:   "容器 CPU 利用率(system) (%)",
				Metric: "irate(container_cpu_system_seconds_total{image!=\"\", image!~\".*pause.*\"}[%RangeVector%] %OFFSET%) * 100",
			},
			{
				Name:   "容器 CPU 利用率(user)(%)",
				Metric: "irate(container_cpu_user_seconds_total{image!=\"\", image!~\".*pause.*\"}[%RangeVector%] %OFFSET%) * 100",
			},
			{
				Name:   "容器 CPU 利用率(整体，值不会大于 100) (%)",
				Metric: "sum( irate(container_cpu_usage_seconds_total{image!=\"\", image!~\".*pause.*\"}[%RangeVector%] %OFFSET%) ) by (pod,namespace,container,image) / sum( container_spec_cpu_quota %OFFSET% /container_spec_cpu_period %OFFSET% ) by (pod,namespace,container,image)",
			},
			{
				Name:   "容器 CPU 利用率(整体，值可能会大于 100) (%)",
				Metric: "irate(container_cpu_usage_seconds_total{image!=\"\", image!~\".*pause.*\"}[%RangeVector%] %OFFSET%) * 100",
			},
			{
				Name:   "容器 CPU 每秒有多少 period (count)",
				Metric: "irate(container_cpu_cfs_periods_total{}[%RangeVector%] %OFFSET%)",
			},
			{
				Name:   "容器 CPU 每秒被 throttle 的 period 量 (count)",
				Metric: "irate(container_cpu_cfs_throttled_periods_total{}[%RangeVector%] %OFFSET%)",
			},
			{
				Name:   "容器 CPU 被 throttle 的比例 (%)",
				Metric: "irate(container_cpu_cfs_throttled_periods_total{}[%RangeVector%] %OFFSET%) / irate(container_cpu_cfs_periods_total{}[%RangeVector%] %OFFSET%) * 100",
			},
			{
				Name:   "容器 IO 每秒写入 byte 量 (bytes/s)",
				Metric: "sum(irate(container_fs_writes_bytes_total[%RangeVector%] %OFFSET%)) by (namespace, pod)",
			},
			{
				Name:   "容器 IO 每秒读取 byte 量 (bytes/s)",
				Metric: "sum(irate(container_fs_reads_bytes_total[%RangeVector%] %OFFSET%)) by (namespace, pod)",
			},
			{
				Name:   "容器 memory 分配失败次数(每秒) (count/s)",
				Metric: "rate(container_memory_failures_total{}[%RangeVector%] %OFFSET%)",
			},
			{
				Name:   "容器 net 每秒发送 bit 量 (bit/s)",
				Metric: "sum(irate(container_network_transmit_bytes_total[%RangeVector%] %OFFSET%)) by (namespace, pod) * 8",
			},
			{
				Name:   "容器 net 每秒接收 bit 量 (bit/s)",
				Metric: "sum(irate(container_network_receive_bytes_total[%RangeVector%] %OFFSET%)) by (namespace, pod) * 8",
			},
		},
	}

	Last1Min            = model.Modifier{RangeVector: "1m", Scale: "1"}
	Last1Hour           = model.Modifier{RangeVector: "1h", Scale: "1"}
	Last1Day            = model.Modifier{RangeVector: "1d", Scale: "1"}
	Last1MinP90         = model.Modifier{RangeVector: "1m", Scale: "1", Quantile: "0.9"}
	Last1HourP90        = model.Modifier{RangeVector: "1h", Scale: "1", Quantile: "0.9"}
	Last1DayP90         = model.Modifier{RangeVector: "1d", Scale: "1", Quantile: "0.9"}
	DayOnDayLast1Min    = model.Modifier{RangeVector: "1m", Scale: "1", DayOnDay: true}
	DayOnDayLast1Hour   = model.Modifier{RangeVector: "1h", Scale: "1", DayOnDay: true}
	WeekOnWeekLast1Min  = model.Modifier{RangeVector: "1m", Scale: "1", WeekOnWeek: true}
	WeekOnWeekLast1Hour = model.Modifier{RangeVector: "1h", Scale: "1", WeekOnWeek: true}
)

var PredefinedDetectExprParts map[string]*model.DetectExprPart

func init() {
	PredefinedDetectExprParts = map[string]*model.DetectExprPart{
		"过去1m的请求延时P90": {
			PredefinedMetric: percentileLantency,
			Modifier:         Last1MinP90,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        0,
		},
		"过去1h的请求延时P90": {
			PredefinedMetric: percentileLantency,
			Modifier:         Last1HourP90,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        1,
		},
		"过去1d的请求延时P90": {
			PredefinedMetric: percentileLantency,
			Modifier:         Last1DayP90,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        2,
		},
		"过去1m的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         Last1Min,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        3,
		},
		"过去1h的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         Last1Hour,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        4,
		},
		"过去1d的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         Last1Day,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        5,
		},
		"昨日同时刻过去1m的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         DayOnDayLast1Min,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        6,
		},
		"昨日同时刻过去1h的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         DayOnDayLast1Hour,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        7,
		},
		"上周同时刻过去1m的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         WeekOnWeekLast1Min,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        8,
		},
		"上周同时刻过去1h的平均请求延时": {
			PredefinedMetric: avgLantency,
			Modifier:         WeekOnWeekLast1Hour,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "latency",
			MetricIdx:        9,
		},
		"过去1m的请求错误率": {
			PredefinedMetric: errorRate,
			Modifier:         Last1Min,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "errorRate",
			MetricIdx:        10,
		},
		"过去1h的请求错误率": {
			PredefinedMetric: errorRate,
			Modifier:         Last1Hour,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "errorRate",
			MetricIdx:        11,
		},
		"过去1d的请求错误率": {
			PredefinedMetric: errorRate,
			Modifier:         Last1Day,
			Group:            kubernetes.MutationAppLabelVal,
			CompareGroup:     "errorRate",
			MetricIdx:        12,
		},
	}

	for group, compareableGroups := range gaugeGroup {
		for compareGroup, comparedMetrics := range compareableGroups {
			for _, metric := range comparedMetrics {
				// 瞬时值
				PredefinedDetectExprParts[metric.Name] = &model.DetectExprPart{
					PredefinedMetric: metric,
					Modifier:         Last1Min,
					Group:            group,
					CompareGroup:     compareGroup,
					MetricIdx:        len(PredefinedDetectExprParts),
				}
				// 昨日同时刻
				PredefinedDetectExprParts["昨日同时刻"+metric.Name] = &model.DetectExprPart{
					PredefinedMetric: metric,
					Modifier:         DayOnDayLast1Min,
					Group:            group,
					CompareGroup:     compareGroup,
					MetricIdx:        len(PredefinedDetectExprParts),
				}
			}
		}
	}

	for group, metrics := range gaugeMetric {
		for _, metric := range metrics {
			compareGroup := len(PredefinedDetectExprParts)
			// 瞬时值
			PredefinedDetectExprParts[metric.Name] = &model.DetectExprPart{
				PredefinedMetric: metric,
				Modifier:         Last1Min,
				Group:            group,
				CompareGroup:     strconv.Itoa(compareGroup),
				MetricIdx:        len(PredefinedDetectExprParts),
			}
			// 昨日同时刻
			PredefinedDetectExprParts["昨日同时刻"+metric.Name] = &model.DetectExprPart{
				PredefinedMetric: metric,
				Modifier:         DayOnDayLast1Min,
				Group:            group,
				CompareGroup:     strconv.Itoa(compareGroup),
				MetricIdx:        len(PredefinedDetectExprParts),
			}
		}
	}

	for group, metrics := range counterMetric {
		for _, metric := range metrics {
			compareGroup := len(PredefinedDetectExprParts)
			// 过去一分钟均值
			PredefinedDetectExprParts["过去1分钟平均"+metric.Name] = &model.DetectExprPart{
				PredefinedMetric: metric,
				Modifier:         Last1Min,
				Group:            group,
				CompareGroup:     strconv.Itoa(compareGroup),
				MetricIdx:        len(PredefinedDetectExprParts),
			}
			// 过去一小时均值
			PredefinedDetectExprParts["过去1小时平均"+metric.Name] = &model.DetectExprPart{
				PredefinedMetric: metric,
				Modifier:         Last1Hour,
				Group:            group,
				CompareGroup:     strconv.Itoa(compareGroup),
				MetricIdx:        len(PredefinedDetectExprParts),
			}
			// 昨日同时刻
			PredefinedDetectExprParts["昨日同时刻过去一分钟"+metric.Name] = &model.DetectExprPart{
				PredefinedMetric: metric,
				Modifier:         DayOnDayLast1Min,
				Group:            group,
				CompareGroup:     strconv.Itoa(compareGroup),
				MetricIdx:        len(PredefinedDetectExprParts),
			}
		}
	}

}

var customMetricInit sync.Once
var predefinedMetricsList []model.PredefinedMetricExpr

// DetectDefectsOptions 提供快速指标模版
func (s *service) DetectDefectsOptions() *response.GetPredefinedDetectExprResponse {
	customMetricInit.Do(func() {
		for key, expr := range PredefinedDetectExprParts {
			expr.CustomMetric = expr.String()
			expr.CustomMetric = strings.ReplaceAll(expr.CustomMetric, "%BucketRange%", s.promRepo.GetRange())
			predefinedMetricsList = append(predefinedMetricsList, model.PredefinedMetricExpr{
				DetectExprPart: *expr,
				Describe:       key,
			})
		}

		sort.SliceStable(predefinedMetricsList, func(i, j int) bool {
			return predefinedMetricsList[i].MetricIdx < predefinedMetricsList[j].MetricIdx
		})
	})
	return &response.GetPredefinedDetectExprResponse{
		PredefinedMetrics: predefinedMetricsList,
	}
}
