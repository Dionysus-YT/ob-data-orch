package exportdomain

import (
	"errors"
	"regexp"
	"strings"

	"ob-data-orch/internal/store"
)

var timestampValuePattern = regexp.MustCompile(`^[A-Za-z0-9 \-/:.'TZ]+$`)

// NeedsExpertMetadata 判断草稿是否需要 v11 参数目录；既有零值草稿继续沿用冻结目录。
func (n Draft) NeedsExpertMetadata() bool {
	f := n.TimestampFormats
	return n.WeakRead || n.EnableHiddenPk || n.AddExtraMessage || n.SequencePolicy != "" ||
		f.TimeValueFormat != "" || f.TimestampValueFormat != "" || f.TimestampTzValueFormat != "" || f.TimestampLtzValueFormat != "" ||
		f.NlsDateFormat != "" || f.NlsTimestampFormat != "" || f.NlsTimestampTzFormat != "" || f.PreserveZeroDatetime
}

// ValidateExpertParameters 校验新增参数的内容与对象边界，不读取数据库或凭据。
// 租户与 sys 材料由 ValidateExpertCompatibility 复验，副本及隐藏主键事实由 Agent 固定探针验证。
func ValidateExpertParameters(n Draft) error {
	if n.WeakRead || n.EnableHiddenPk {
		if n.ContentKind == "DDL_ONLY" || n.ScopeKind == "QUERY_RESULT" || n.QuerySql != "" {
			return errors.New("export read options require object data scope")
		}
	}
	if n.EnableHiddenPk && n.ScopeKind != "ALL" && n.ObjectType != "TABLE" {
		return errors.New("hidden primary key requires table data scope")
	}
	if n.AddExtraMessage && (n.ContentKind == "DATA_ONLY" || !n.hasObjectType("TABLE")) {
		return errors.New("extra definition information requires table ddl scope")
	}
	if n.SequencePolicy != "" {
		if (n.SequencePolicy != "PRESERVE" && n.SequencePolicy != "RESTART") || n.ContentKind == "DATA_ONLY" || !n.hasObjectType("SEQUENCE") {
			return errors.New("sequence policy requires sequence ddl scope and a supported policy")
		}
	}
	f := n.TimestampFormats
	for _, value := range []string{f.DateValueFormat, f.TimeValueFormat, f.DateTimeValueFormat, f.TimestampValueFormat, f.TimestampTzValueFormat, f.TimestampLtzValueFormat, f.NlsDateFormat, f.NlsTimestampFormat, f.NlsTimestampTzFormat} {
		if value != "" && (len(value) > 256 || strings.TrimSpace(value) != value || !timestampValuePattern.MatchString(value)) {
			return errors.New("export timestamp format contains unsupported characters")
		}
	}
	if f != (store.TimestampFormatConfig{}) && n.ContentKind == "DDL_ONLY" {
		return errors.New("export timestamp formats require data content")
	}
	valueFormats := f.DateValueFormat != "" || f.TimeValueFormat != "" || f.DateTimeValueFormat != "" || f.TimestampValueFormat != "" || f.TimestampTzValueFormat != "" || f.TimestampLtzValueFormat != "" || f.PreserveZeroDatetime
	if valueFormats && n.Format != "CSV" && n.Format != "CUT" {
		return errors.New("export timestamp value formats require csv or cut")
	}
	return nil
}

// ValidateExpertCompatibility 仅使用控制面读取的当前数据源事实，浏览器不能自报兼容模式或 sys 资格。
func ValidateExpertCompatibility(n Draft, mode string, sysConfigured bool) error {
	f := n.TimestampFormats
	if f != (store.TimestampFormatConfig{}) && mode != "MYSQL" && mode != "ORACLE" {
		return errors.New("export timestamp compatibility is unknown")
	}
	if mode != "MYSQL" && (f.TimeValueFormat != "" || f.DateTimeValueFormat != "" || f.PreserveZeroDatetime) {
		return errors.New("mysql timestamp options require mysql compatibility")
	}
	if mode != "ORACLE" && (f.TimestampTzValueFormat != "" || f.TimestampLtzValueFormat != "" || f.NlsDateFormat != "" || f.NlsTimestampFormat != "" || f.NlsTimestampTzFormat != "") {
		return errors.New("oracle timestamp options require oracle compatibility")
	}
	if n.AddExtraMessage && !sysConfigured {
		return errors.New("extra definition information requires current sys credentials")
	}
	return nil
}

func (n Draft) hasObjectType(kind string) bool {
	return n.ScopeKind == "ALL" || n.ObjectType == kind || len(n.ObjectsByType[kind]) != 0
}
