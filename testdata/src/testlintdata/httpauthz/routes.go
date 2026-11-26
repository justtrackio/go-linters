package httpauthz

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/gosoline-project/authz/httpserver"
	hs "github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
)

const reason = "public " + "health check"

var dynamicReason = "not constant"

func raw(*gin.Context)                     {}
func makeHandler() *int                    { return nil }
func otherFactory() hs.RegisterFactoryFunc { return nil }
func register(*hs.Router, *int)            {}
func GET(string, ...gin.HandlerFunc)       {}

func routes(r *hs.Router, g *gin.RouterGroup, engine *gin.Engine, mux *http.ServeMux) {
	hs.RunServers(nil)
	group := r.Group("/v1").Group("")
	group.Use(raw)
	group.GET("/a", auth.Bind[int, int](nil, nil, nil))
	group.POST("/b", auth.BindWithOptions[int, int](nil, nil, nil, nil))
	group.Handle("GET", "/c", auth.BindN[int](nil, nil, nil))
	group.PUT("/prepared", auth.BindPrepared[int, int](nil, nil, nil))
	group.DELETE("/public", auth.Exempt(reason, raw))
	group.HandleWith(hs.With[int](makeHandler, func(r *hs.Router, h *int) {
		r.GET("/nested", auth.Exempt("public", raw))
	}))
	GET("unrelated", raw)

	r.GET("/raw", raw)                                 // want "exactly one direct"
	r.GET("/multiple", raw, auth.Exempt(reason, raw))  // want "exactly one direct"
	r.GET("/extra", auth.Exempt(reason, raw), raw)     // want "exactly one direct"
	r.GET("/blank", auth.Exempt(" \t", raw))           // want "exactly one direct"
	r.GET("/dynamic", auth.Exempt(dynamicReason, raw)) // want "exactly one direct"
	r.GET("/nil", auth.Exempt(reason, nil))            // want "exactly one direct"
	v := auth.Exempt(reason, raw)
	r.GET("/indirect", v) // want "exactly one direct"
	handlers := []gin.HandlerFunc{auth.Exempt(reason, raw)}
	r.GET("/spread", handlers...)                                        // want "exactly one direct"
	r.Handle("GET", "/raw", raw)                                         // want "exactly one direct"
	r.HandleWith(otherFactory())                                         // want "HandleWith requires"
	r.HandleWith(hs.With(makeHandler, register))                         // want "HandleWith requires"
	r.HandleWith(sqlh.WithCrudHandlers())                                // want "HandleWith requires" "SQLH automatic"
	_ = r.GET                                                            // want "opaque HTTP registration"
	_ = (*hs.Router).GET                                                 // want "opaque HTTP registration"
	(*hs.Router).GET(r, "/method-expression", raw)                       // want "opaque HTTP registration"
	g.GET("/gin", raw)                                                   // want "raw Gin route"
	engine.POST("/gin", raw)                                             // want "raw Gin route"
	_ = g.GET                                                            // want "opaque HTTP registration"
	mux.HandleFunc("/http", func(http.ResponseWriter, *http.Request) {}) // want "independent net/http"
	http.Handle("/http", http.NotFoundHandler())                         // want "independent net/http"
	_ = http.ListenAndServe                                              // want "opaque HTTP serving"
	_ = gin.WrapH(http.NotFoundHandler())                                // want "external HTTP handlers"
	var routes gin.IRoutes = g
	routes.GET("/interface", raw)              // want "raw Gin route"
	engine.Run()                               // want "independent Gin serving"
	engine.HandleContext(nil)                  // want "independent Gin serving"
	_ = engine.HandleContext                   // want "opaque HTTP registration"
	http.ListenAndServe(":8080", nil)          // want "independent net/http"
	http.NotFoundHandler().ServeHTTP(nil, nil) // want "external HTTP handler serving"
	fmt.Sprint(r)                              // want "external registration helper"
	_ = hs.CreateEmbeddedStaticServe()         // want "middleware-based static serving"
	_ = hs.NewWithInterfaces()                 // want "externally configured router"
	r.Use(hs.Bind(raw))                        // want "route handlers must be mounted as routes"
	r.Use(auth.Exempt(reason, raw))            // want "route handlers must be mounted as routes"
	aliasServeHTTP{}.ServeHTTP(nil, nil)       // want "external HTTP handler serving"
}

type aliasedRequest = *http.Request

type aliasServeHTTP struct{}

func (aliasServeHTTP) ServeHTTP(http.ResponseWriter, aliasedRequest) {
}
