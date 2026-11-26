package httpserver

import "github.com/gin-gonic/gin"

type (
	Router              struct{}
	RegisterFactoryFunc func()
)

func (*Router) GET(string, ...gin.HandlerFunc)                                      {}
func (*Router) POST(string, ...gin.HandlerFunc)                                     {}
func (*Router) PUT(string, ...gin.HandlerFunc)                                      {}
func (*Router) DELETE(string, ...gin.HandlerFunc)                                   {}
func (*Router) PATCH(string, ...gin.HandlerFunc)                                    {}
func (*Router) OPTIONS(string, ...gin.HandlerFunc)                                  {}
func (*Router) Handle(string, string, ...gin.HandlerFunc)                           {}
func (*Router) HandleWith(RegisterFactoryFunc)                                      {}
func (r *Router) Group(string) *Router                                              { return r }
func (*Router) Use(...gin.HandlerFunc)                                              {}
func With[H any](factory func() *H, register func(*Router, *H)) RegisterFactoryFunc { return nil }
func RunServers(any)                                                                {}
func Bind(any) gin.HandlerFunc                                                      { return nil }
func CreateEmbeddedStaticServe() func()                                             { return nil }
func NewWithInterfaces() *int                                                       { return nil }
