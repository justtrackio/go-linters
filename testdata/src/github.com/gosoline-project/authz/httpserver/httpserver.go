package httpserver

import "github.com/gin-gonic/gin"

func Bind[I, O any](a, policy, handler any) gin.HandlerFunc { return nil }
func BindWithOptions[I, O any](a, policy, handler, options any) gin.HandlerFunc { return nil }
func BindN[O any](a, policy, handler any) gin.HandlerFunc { return nil }
func BindPrepared[I, O any](a, policy, handler any) gin.HandlerFunc { return nil }
func Exempt(reason string, handler gin.HandlerFunc) gin.HandlerFunc { return handler }
