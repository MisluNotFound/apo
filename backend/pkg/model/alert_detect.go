package model

import (
	"fmt"
	"strings"
)

type Modifier struct {
	RangeVector string `json:"rangeVector,omitempty"` // 1m/1h/1d
	Scale       string `json:"scale,omitempty"`       // 倍率
	Quantile    string `json:"quantile,omitempty"`    // 百分位数
	DayOnDay    bool   `json:"dayOnDay"`              // 使用昨日同时段数据
	WeekOnWeek  bool   `json:"weekOnWeek"`            // 使用上周同时段数据
}

// PredefinedMetrics 预定义指标
type PredefinedMetric struct {
	Name   string `json:"name"`
	Metric string `json:"metric"`
}

// DetectExpr 检测表达式
type DetectExprPart struct {
	PredefinedMetric
	Modifier Modifier `json:"modifier"`

	// 用于记录指标所属的告警事件组
	Group string `json:"group"`

	// 用于记录可以相互比较的检测表达式
	CompareGroup string `json:"compareGroup"`

	// 常量表达式
	CustomMetric string `json:"customMetric"`
}

type PredefinedMetricExpr struct {
	DetectExprPart

	Describe string `json:"describe"`
}

func (p *DetectExprPart) String() string {
	if p == nil {
		return ""
	}

	if len(p.CustomMetric) > 0 {
		return p.CustomMetric
	}

	if len(p.PredefinedMetric.Name) == 0 {
		// 非预定义指标,直接用原始值
		return p.PredefinedMetric.Metric
	}

	rawMetric := p.PredefinedMetric.Metric
	if len(p.Modifier.RangeVector) > 0 {
		rawMetric = strings.ReplaceAll(rawMetric, "%RangeVector%", p.Modifier.RangeVector)
	}
	if len(p.Modifier.Quantile) > 0 {
		rawMetric = strings.ReplaceAll(rawMetric, "%Quantile%", p.Modifier.Quantile)
	}

	if p.Modifier.DayOnDay {
		rawMetric = strings.ReplaceAll(rawMetric, "%OFFSET%", "offset 24h")
	} else if p.Modifier.WeekOnWeek {
		rawMetric = strings.ReplaceAll(rawMetric, "%OFFSET%", "offset 7d")
	} else {
		rawMetric = strings.ReplaceAll(rawMetric, "%OFFSET%", "")
	}

	if len(p.Modifier.Scale) > 0 {
		rawMetric = fmt.Sprintf("(%s) * %s", rawMetric, p.Modifier.Scale)
	}

	return rawMetric
}
