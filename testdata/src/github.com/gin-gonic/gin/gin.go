package gin

type HandlerFunc func(*Context)
type Context struct{}
type RouterGroup struct{}
type Engine struct{ RouterGroup }
type IRoutes interface { GET(string, ...HandlerFunc) IRoutes }
type IRouter interface { IRoutes }
func (*RouterGroup) GET(string, ...HandlerFunc) IRoutes { return nil }
func (*Engine) Run(...string) error { return nil }
func (*RouterGroup) POST(string, ...HandlerFunc) IRoutes { return nil }
func (*RouterGroup) Handle(string, string, ...HandlerFunc) IRoutes { return nil }
func WrapH(any) HandlerFunc { return nil }
func WrapF(any) HandlerFunc { return nil }
