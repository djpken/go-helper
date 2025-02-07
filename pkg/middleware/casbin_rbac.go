package middleware

import (
	"github.com/djpken/go-helper/pkg/resp"
	"github.com/djpken/go-helper/pkg/tracing"
	"github.com/gin-gonic/gin"
	"strings"
	"sync"
)

func Casbin(options ...func(*CasbinOptions)) gin.HandlerFunc {
	ops := getCasbinOptionsOrSetDefault(nil)
	for _, f := range options {
		f(ops)
	}
	if ops.Enforcer == nil {
		panic("casbin Enforcer is empty")
	}
	if ops.Enforcer == nil {
		panic("casbin getCurrentUser is empty")
	}
	return func(c *gin.Context) {
		ctx := tracing.RealCtx(c)
		_, span := tracer.Start(ctx, tracing.Name(tracing.Middleware, "Casbin"))
		var pass bool
		defer func() {
			if !pass {
				span.End()
			}
		}()
		// get role.key as subject
		sub := ops.getCurrentUser(c)
		// request path as object
		obj := strings.Replace(c.Request.URL.Path, "/"+ops.urlPrefix, "", 1)
		// request method as action
		act := c.Request.Method
		if !check(sub.RoleKeyword, obj, act, *ops) {
			ops.failWithCode(resp.Forbidden)
			return
		}
		span.End()
		pass = true
		c.Next()
	}
}

var checkLock sync.Mutex

func check(sub, obj, act string, ops CasbinOptions) bool {
	checkLock.Lock()
	defer checkLock.Unlock()
	pass, _ := ops.Enforcer.Enforce(sub, obj, act)
	return pass
}
