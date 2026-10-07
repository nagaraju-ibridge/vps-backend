package main

import (
	"fmt"
	"gofr.dev/pkg/gofr"
)

type AppReq struct {
	Name        string `json:"name"`
	MonitorPort int    `json:"monitor_port,omitempty"`
}

func main() {
	app := gofr.New()

	app.POST("/test", func(ctx *gofr.Context) (interface{}, error) {
		var req AppReq
		if err := ctx.Bind(&req); err != nil {
			return nil, err
		}
		fmt.Printf("Parsed port: %v\n", req.MonitorPort)
		return req, nil
	})

	app.Run()
}
