package pprof

import (
	"net/http"
	// Init PPROF
	_ "net/http/pprof"

	"github.com/labstack/echo/v4"
)

// GetPPROF returns a handler that serves the /debug/pprof/ URLs
func GetPPROF(grp *echo.Group) {
	grp.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
}
