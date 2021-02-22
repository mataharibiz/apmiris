# APM IRIS

This package is for middleware kataras iris, so you can add this to your application and logging to APM.

Reference : [issue](https://github.com/elastic/apm-agent-go/issues/891)

## How to use
```bash
    go get -u github.com/mataharibiz/apmiris
```
### Setup ENV
first you need too setup basic environment variable, for more advance setup please refer to [apm configuration](https://www.elastic.co/guide/en/apm/agent/go/current/configuration.html).
```bash
export ELASTIC_APM_SERVICE_NAME=your-app-name
export ELASTIC_APM_SERVER_URL=http://your.apm.server:port
export ELASTIC_APM_SECRET_TOKEN=apm-token-app
```

### Example Default Recover from PANIC
this example doesn't need context
```go
package main

import (
    "fmt"
	"github.com/mataharibiz/apmiris"
	"github.com/kataras/iris/v12"
)

func main() {
    app := iris.New()

    app.Get("/test-panic", func(context iris.Context) {
        defer func() {
            apmiris.RecoverApmDefault("test panic")

            if err := recover(); err != nil {
                context.StatusCode(iris.StatusOK)
                _, _ = context.JSON(iris.Map{
                    "message": "You just recovered",
                })
            }
        }()

        panic("panic test")
        return
    })
        
    _ = app.Listen(":8080")
}
```

### Example Default Send Error
```go
package main

import (
    "fmt"
	"github.com/mataharibiz/apmiris"
	"github.com/kataras/iris/v12"
)

func main() {
    app := iris.New()

    app.Get("/test-error", func(ctx iris.Context) {
        apmiris.SendErrorApmDefault(fmt.Errorf("this is error"))

        ctx.StatusCode(iris.StatusNotFound)
        _, _ = ctx.JSON(iris.Map{
            "message": "Testing error",
        })
        return
    })
        
    _ = app.Listen(":8080")
}
```

### Example Code Middleware
```go
package main

import (
    "fmt"
	"github.com/mataharibiz/apmiris"
	"github.com/kataras/iris/v12"
)

func main() {
    app := iris.New()
    
    // setup middleware + user data so your logs have user data
    // in this example my application using redis session, but you can change it
    app.Use(apmiris.Middleware(app, func(ctx iris.Context) (userData *apmiris.GetUserData) {
        sessionRedis := GetRedisSessionConnection()
        s := sessionRedis.Start(ctx)
        
        userModel := s.GetString("auth")
        var dataUser map[string]interface{}
        _ = json.Unmarshal([]byte(userModel), &dataUser)
        
        user := &apmiris.UserData{
            UserID:    fmt.Sprintf("%v", dataUser["id"]),
            UserName:  fmt.Sprintf("%v", dataUser["username"]),
            UserEmail: fmt.Sprintf("%v", dataUser["email"]),
        }
        
        return user
    }))
        
    app.Get("/test", func(ctx iris.Context) {
        ctx.StatusCode(iris.StatusOK)
        _, _ = ctx.JSON(iris.Map{
            "message": "You've been mixing with the wrong crowd.",
        })
        return
    })
        
    _ = app.Listen(":8080")
}
```

### Example Code Error
```go
package main

import (
    "fmt"
	"github.com/mataharibiz/apmiris"
	"github.com/kataras/iris/v12"
)

func main() {
    app := iris.New()
    
    app.Get("/test", func(ctx iris.Context) {
        apmiris.NewApmError(nil, ctx).
            SetAction("test error", "controller.role"). 
            SetLevelError("lowest"). // you can use custom level error
            SetTitle("Error when get input json"). // title for your error
            SetAdditionalData("additional data").
            SendError(fmt.Errorf("this is the error"))

        ctx.StatusCode(iris.StatusOK)
        _, _ = ctx.JSON(iris.Map{
            "message": "You've been mixing with the wrong crowd.",
        })
        return
    })
        
    _ = app.Listen(":8080")
}
```