package main

import (
	"github.com/buildset/buildset/app/webapp"
	"github.com/buildset/buildset/pkg/serve"
)

func main() { serve.Main(webapp.Run) }
