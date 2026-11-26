package httpauthz

import hs "github.com/gosoline-project/httpserver"

func testOnly(r *hs.Router) { r.GET("/test", raw) }
