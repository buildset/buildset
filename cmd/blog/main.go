package main

import (
	"github.com/buildset/buildset/app"
	"github.com/buildset/buildset/pkg/serve"
)

func main() { serve.Main(app.Run) }
