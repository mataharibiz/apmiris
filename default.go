package apmiris

import (
	"go.elastic.co/apm"
	"time"
)

func RecoverApmDefault(recoverType string) {
	if err := recover(); err != nil {
		tracer := apm.DefaultTracer
		tx := tracer.StartTransaction("[PANIC]", recoverType)

		span := tx.StartSpanOptions("[PANIC]", "recovered.panic", apm.SpanOptions{Start: time.Now()})
		span.SetStacktrace(1)
		defer span.End()

		e := tracer.Recovered(err)
		e.SetTransaction(tx)
		e.Send()
	}
}

func SendErrorApmDefault(err error) {
	tracer := apm.DefaultTracer
	tx := tracer.StartTransaction("[ERROR]", err.Error())
	defer tx.End()

	span := tx.StartSpanOptions("[ERROR]", "", apm.SpanOptions{Start: time.Now()})
	span.SetStacktrace(1)
	defer span.End()

	e := tracer.NewError(err)
	e.SetTransaction(tx)
	e.SetSpan(span)
	e.Send()
}
