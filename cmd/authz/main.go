package main

import (
	"github.com/buildset/buildset/app/authzapp"
	"github.com/buildset/buildset/pkg/serve"
)

func main() { serve.Main(authzapp.Run) }
