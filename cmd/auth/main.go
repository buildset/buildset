package main

import (
	"github.com/buildset/buildset/app/authapp"
	"github.com/buildset/buildset/pkg/serve"
)

func main() { serve.Main(authapp.Run) }
