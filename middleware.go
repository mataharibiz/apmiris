package apmiris

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"

	"github.com/kataras/iris/v12"

	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmhttp"
)

// Middleware returns a new Kataras/Iris middleware handler for tracing
// requests and reporting errors.
//
// This middleware will recover and report panics, so it can
// be used instead of the standard gin.Recovery middleware.
//
// By default, the middleware will use apm.DefaultTracer.
// Use WithTracer to specify an alternative tracer.
func Middleware(engine *iris.Application, getUserData GetUserData, opts ...Option) iris.Handler {
	m := &middleware{
		engine:      engine,
		tracer:      apm.DefaultTracer,
		getUserData: getUserData,
	}

	for _, opt := range opts {
		opt(m)
	}

	if m.requestIgnorer == nil {
		m.requestIgnorer = apmhttp.NewDynamicServerRequestIgnorer(m.tracer)
	}

	return m.handle
}

type middleware struct {
	engine         *iris.Application
	tracer         *apm.Tracer
	requestIgnorer apmhttp.RequestIgnorerFunc

	setRouteMapOnce sync.Once
	routeMap        map[string]map[string]routeInfo
	getUserData     GetUserData
}

type GetUserData func(ctx iris.Context) (userData *UserData)

type routeInfo struct {
	transactionName string // e.g. "GET /foo"
}

type UserData struct {
	UserID    string
	UserEmail string
	UserName  string
}

func (m *middleware) handle(irisCtx iris.Context) {
	if !m.tracer.Recording() || m.requestIgnorer(irisCtx.Request()) {
		irisCtx.Next()
		return
	}

	m.setRouteMapOnce.Do(func() {
		routes := m.engine.GetRoutes()
		rm := make(map[string]map[string]routeInfo)
		for _, r := range routes {
			mm := rm[r.Method]
			if mm == nil {
				mm = make(map[string]routeInfo)
				rm[r.Method] = mm
			}

			mm[r.MainHandlerName] = routeInfo{
				transactionName: r.Method + " " + r.Path,
			}
		}

		m.routeMap = rm
	})

	var requestName string
	handlerName := irisCtx.GetCurrentRoute().MainHandlerName()
	if routeInfo, ok := m.routeMap[irisCtx.Method()][handlerName]; ok {
		requestName = routeInfo.transactionName
	} else {
		requestName = apmhttp.UnknownRouteRequestName(irisCtx.Request())
	}

	tx, req := apmhttp.StartTransaction(m.tracer, requestName, irisCtx.Request())
	irisCtx.ResetRequest(req)
	defer tx.End()

	userData := m.getUserData(irisCtx)
	if userData != nil {
		tx.Context.SetUserID(userData.UserID)
		tx.Context.SetUserEmail(userData.UserEmail)
		tx.Context.SetUsername(userData.UserName)
	}

	body := m.tracer.CaptureHTTPRequestBody(irisCtx.Request())
	defer func() {
		if err := recover(); err != nil {
			if irisCtx.IsStopped() {
				return
			}

			var errorTrace string
			for i := 1; ; i++ {
				_, f, l, got := runtime.Caller(i)
				if !got {
					break
				}

				errorTrace += fmt.Sprintf("%s:%d\n", f, l)
			}

			// when stack finishes
			logMessage := fmt.Sprintf("Recovered from a route's Handler('%s')\n", irisCtx.GetCurrentRoute().MainHandlerName())
			logMessage += fmt.Sprintf("At Request: %s\n", getRequestLogs(irisCtx))
			logMessage += fmt.Sprintf("Trace: %s\n", err)
			logMessage += fmt.Sprintf("\n%s", errorTrace)
			irisCtx.Application().Logger().Warn(logMessage)

			irisCtx.StatusCode(http.StatusInternalServerError)
			irisCtx.StopExecution()

			e := m.tracer.Recovered(err)
			e.SetTransaction(tx)
			setContext(&e.Context, irisCtx, body)
			e.Send()
		}

		irisCtx.ResponseWriter().WriteHeader(irisCtx.GetStatusCode())
		tx.Result = apmhttp.StatusCodeResult(irisCtx.ResponseWriter().StatusCode())

		if tx.Sampled() {
			setContext(&tx.Context, irisCtx, body)
		}

		body.Discard()
	}()
	irisCtx.Next()
}

func setContext(apmCtx *apm.Context, irisCtx iris.Context, body *apm.BodyCapturer) {
	apmCtx.SetFramework("iris", iris.Version)
	apmCtx.SetHTTPRequest(irisCtx.Request())
	apmCtx.SetHTTPRequestBody(body)
	apmCtx.SetHTTPStatusCode(irisCtx.ResponseWriter().StatusCode())
	apmCtx.SetHTTPResponseHeaders(irisCtx.ResponseWriter().Header())
}

// Option sets options for tracing.
type Option func(*middleware)

// WithTracer returns an Option which sets t as the tracer
// to use for tracing server requests.
func WithTracer(t *apm.Tracer) Option {
	if t == nil {
		panic("t == nil")
	}
	return func(m *middleware) {
		m.tracer = t
	}
}

func getRequestLogs(ctx iris.Context) string {
	var status, ip, method, path string
	status = strconv.Itoa(ctx.GetStatusCode())
	path = ctx.Path()
	method = ctx.Method()
	ip = ctx.RemoteAddr()
	// the date should be logged by iris' Logger, so we skip them
	return fmt.Sprintf("%v %s %s %s", status, path, method, ip)
}
