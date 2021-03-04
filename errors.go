package apmiris

import (
	"fmt"
	"time"

	"github.com/kataras/iris/v12"
	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmhttp"
)

type apmError struct {
	tracer         *apm.Tracer
	requestIgnorer apmhttp.RequestIgnorerFunc
	userData       *UserData
	irisCtx        iris.Context
	additionalData interface{}

	errorLogs *apm.ErrorLogRecord
	spans     *span
	culprit   string
}

// Option sets options for tracing.
type OptionError func(*apmError)

type span struct {
	SpanName string
	SpanType string
}

func NewApmError(userData *UserData, ctx iris.Context, opts ...OptionError) *apmError {
	base := &apmError{
		tracer:    apm.DefaultTracer,
		userData:  userData,
		irisCtx:   ctx,
		errorLogs: &apm.ErrorLogRecord{},
		spans:     &span{},
	}

	for _, opt := range opts {
		opt(base)
	}

	if base.requestIgnorer == nil {
		base.requestIgnorer = apmhttp.NewDynamicServerRequestIgnorer(base.tracer)
	}

	return base
}

// SetAdditionalData set additional data to label span trace, please use json data type
func (base *apmError) SetAdditionalData(value interface{}) *apmError {
	base.additionalData = value
	return base
}

func (base *apmError) SetTitle(title string) *apmError {
	base.errorLogs.Message = title
	return base
}

func (base *apmError) SetLevelError(level string) *apmError {
	base.errorLogs.Level = level
	return base
}

func (base *apmError) SetCulprit(culprit string) *apmError {
	base.culprit = culprit
	return base
}

// SetAction If the span type contains two dots, they are assumed to separate the span type, subtype, and action;
// a single dot separates span type and subtype, and the action will not be set.
func (base *apmError) SetAction(spanName string, spanType string) *apmError {
	base.spans = &span{
		SpanName: spanName,
		SpanType: spanType,
	}

	return base
}

func (base *apmError) SendError(errorDetails error) {
	irisCtx := base.irisCtx
	if !base.tracer.Recording() || base.requestIgnorer(irisCtx.Request()) {
		return
	}

	body := base.tracer.CaptureHTTPRequestBody(irisCtx.Request())
	requestName := fmt.Sprintf("%s %s", irisCtx.GetCurrentRoute().Method(), irisCtx.GetCurrentRoute().Path())

	tx, req := apmhttp.StartTransaction(base.tracer, requestName, irisCtx.Request())
	irisCtx.ResetRequest(req)
	defer tx.End()

	userData := base.userData
	if userData != nil {
		tx.Context.SetUserID(userData.UserID)
		tx.Context.SetUserEmail(userData.UserEmail)
		tx.Context.SetUsername(userData.UserName)
	}

	var spanName, spanType string
	if base.spans != nil {
		spanName = base.spans.SpanName
		spanType = base.spans.SpanType
	}

	span := tx.StartSpanOptions(spanName, spanType, apm.SpanOptions{Start: time.Now()})
	span.SetStacktrace(1)
	defer span.End()

	if base.additionalData != nil {
		tx.Context.SetLabel("additional_data", base.additionalData)
		span.Context.SetLabel("additional_data", base.additionalData)
	}

	errorLogs := base.errorLogs
	errorLogs.Error = errorDetails

	e := base.tracer.NewErrorLog(*errorLogs)
	e.SetTransaction(tx)
	e.SetSpan(span)
	e.SetStacktrace(1)

	if e.Culprit == "" && base.culprit != "" {
		e.Culprit = base.culprit
	}

	setContext(&e.Context, irisCtx, body)
	e.Send()
}
