package main

import (
	"context"
	"os"

	"panelofexperts/internal/app"
)

func main() {
	os.Exit(app.NewDefault().Run(context.Background(), os.Args[1:]))
}
