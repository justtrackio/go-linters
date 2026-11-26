package gin

type (
	HandlerFunc func(*Context)
	Context     struct{}
	RouterGroup struct{}
	Engine      struct{ RouterGroup }
	IRoutes     interface {
		GET(string, ...HandlerFunc) IRoutes
	}
)
type IRouter interface{ IRoutes }

func (*RouterGroup) GET(string, ...HandlerFunc) IRoutes            { return nil }
func (*Engine) Run(...string) error                                { return nil }
func (*Engine) HandleContext(*Context)                             {}
func (*RouterGroup) POST(string, ...HandlerFunc) IRoutes           { return nil }
func (*RouterGroup) Handle(string, string, ...HandlerFunc) IRoutes { return nil }
func WrapH(any) HandlerFunc                                        { return nil }
func WrapF(any) HandlerFunc                                        { return nil }
