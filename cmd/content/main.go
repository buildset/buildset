package main

import (
	"github.com/buildset/buildset/app/contentapp"
	"github.com/buildset/buildset/pkg/serve"
)

func main() { serve.Main(contentapp.Run) }
