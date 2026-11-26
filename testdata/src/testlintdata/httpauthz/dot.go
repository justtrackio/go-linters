package httpauthz

import (
	. "github.com/gosoline-project/authz/httpserver"
	. "github.com/gosoline-project/httpserver"
)

func dot(r *Router) {
	r.GET("/dot", Bind[int, int](nil, nil, nil))
	r.GET("/dot-exempt", Exempt("public", raw))
}

type unrelated struct{}
func (unrelated) GET(string) {}
func unrelatedRoute(u unrelated) { u.GET("not a route") }
